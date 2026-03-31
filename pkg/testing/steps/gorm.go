//go:build e2e

package steps

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	messages "github.com/cucumber/messages/go/v21"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type GormOptions struct {
	Registry map[string]reflect.Type
	Models   []any
}

type GormSteps struct {
	db   *gorm.DB
	opts GormOptions
}

func NewGormSteps(db *gorm.DB, opts GormOptions) *GormSteps {
	return &GormSteps{db: db, opts: opts}
}

func (g *GormSteps) DB() *gorm.DB {
	return g.db
}

func (g *GormSteps) TruncateAll() error {
	tables := g.allTableNames()
	return g.db.Transaction(func(tx *gorm.DB) error {
		for _, t := range tables {
			if err := tx.Exec("TRUNCATE TABLE " + t + " CASCADE").Error; err != nil {
				return fmt.Errorf("truncate %s: %w", t, err)
			}
		}
		return nil
	})
}

func (g *GormSteps) TruncateTable(table string) error {
	return g.db.Exec("TRUNCATE TABLE " + table + " CASCADE").Error
}

func (g *GormSteps) SeedTable(table string, data *godog.Table) error {
	entityType, err := g.resolveType(table)
	if err != nil {
		return err
	}

	if len(data.Rows) < 2 {
		return fmt.Errorf("table %q: need header + at least one data row", table)
	}

	headers := headerNames(data.Rows[0])

	for _, row := range data.Rows[1:] {
		obj := reflect.New(entityType).Interface()
		elem := reflect.ValueOf(obj).Elem()

		for i, cell := range row.Cells {
			if i >= len(headers) || cell.Value == "" {
				continue
			}

			field := elem.FieldByName(headers[i])
			if !field.IsValid() {
				return fmt.Errorf("field %q not found on %s", headers[i], entityType.Name())
			}

			if err := setFieldValue(field, cell.Value); err != nil {
				return fmt.Errorf("set %s.%s = %q: %w", entityType.Name(), headers[i], cell.Value, err)
			}
		}

		if err := g.db.Table(table).Create(obj).Error; err != nil {
			return fmt.Errorf("insert into %s: %w", table, err)
		}
	}

	return nil
}

func (g *GormSteps) AssertContains(table string, data *godog.Table) error {
	entityType, err := g.resolveType(table)
	if err != nil {
		return g.assertContainsRaw(table, data)
	}

	if len(data.Rows) < 2 {
		return fmt.Errorf("table %q: need header + at least one data row", table)
	}

	headers := headerNames(data.Rows[0])

	for rowIdx, row := range data.Rows[1:] {
		conditions := g.buildConditions(entityType, headers, row)
		tx := g.db.Table(table).Model(reflect.New(entityType).Interface())

		var count int64
		if err := tx.Where(conditions).Count(&count).Error; err != nil {
			return fmt.Errorf("query %s: %w", table, err)
		}

		if count == 0 {
			return fmt.Errorf("row %d not found in %s: %s", rowIdx+1, table, formatConditions(headers, row))
		}
	}

	return nil
}

func (g *GormSteps) AssertNotContains(table string, data *godog.Table) error {
	entityType, err := g.resolveType(table)
	if err != nil {
		return fmt.Errorf("table %q not in registry: %w", table, err)
	}

	if len(data.Rows) < 2 {
		return fmt.Errorf("table %q: need header + at least one data row", table)
	}

	headers := headerNames(data.Rows[0])

	for rowIdx, row := range data.Rows[1:] {
		conditions := g.buildConditions(entityType, headers, row)
		tx := g.db.Table(table).Model(reflect.New(entityType).Interface())

		var count int64
		if err := tx.Where(conditions).Count(&count).Error; err != nil {
			return fmt.Errorf("query %s: %w", table, err)
		}

		if count > 0 {
			return fmt.Errorf("row %d unexpectedly found in %s: %s", rowIdx+1, table, formatConditions(headers, row))
		}
	}

	return nil
}

func (g *GormSteps) AssertCount(expected int, table string) error {
	var count int64

	if entityType, err := g.resolveType(table); err == nil {
		g.db.Model(reflect.New(entityType).Interface()).Count(&count)
	} else {
		g.db.Table(table).Count(&count)
	}

	if int(count) != expected {
		return fmt.Errorf("expected %d rows in %s, got %d", expected, table, count)
	}
	return nil
}

func (g *GormSteps) AssertCountWhere(expected int, table string, data *godog.Table) error {
	entityType, err := g.resolveType(table)
	if err != nil {
		return fmt.Errorf("table %q not in registry: %w", table, err)
	}

	if len(data.Rows) < 2 {
		return fmt.Errorf("need header + data row")
	}

	headers := headerNames(data.Rows[0])
	conditions := g.buildConditions(entityType, headers, data.Rows[1])

	var count int64
	g.db.Model(reflect.New(entityType).Interface()).Where(conditions).Count(&count)

	if int(count) != expected {
		return fmt.Errorf("expected %d rows in %s matching %v, got %d", expected, table, conditions, count)
	}
	return nil
}

func (g *GormSteps) DeleteFrom(table string, data *godog.Table) error {
	entityType, err := g.resolveType(table)
	if err != nil {
		return fmt.Errorf("table %q not in registry: %w", table, err)
	}

	if len(data.Rows) < 2 {
		return fmt.Errorf("need header + data row")
	}

	headers := headerNames(data.Rows[0])

	for _, row := range data.Rows[1:] {
		conditions := g.buildConditions(entityType, headers, row)
		if err := g.db.Model(reflect.New(entityType).Interface()).Where(conditions).Delete(reflect.New(entityType).Interface()).Error; err != nil {
			return fmt.Errorf("delete from %s: %w", table, err)
		}
	}

	return nil
}

func (g *GormSteps) UpdateTable(table string, data *godog.Table) error {
	entityType, err := g.resolveType(table)
	if err != nil {
		return fmt.Errorf("table %q not in registry: %w", table, err)
	}

	if len(data.Rows) < 2 {
		return fmt.Errorf("need header + data row")
	}

	headers := headerNames(data.Rows[0])
	whereCols := make(map[string]any)
	setCols := make(map[string]any)

	for i, cell := range data.Rows[1].Cells {
		if i >= len(headers) {
			break
		}

		header := headers[i]
		parts := strings.SplitN(header, ".", 2)
		if len(parts) != 2 {
			return fmt.Errorf("header %q must be where.Field or set.Field", header)
		}

		prefix, fieldName := parts[0], parts[1]
		colName := g.columnName(entityType, fieldName)
		val := parseValue(cell.Value)

		switch prefix {
		case "where":
			whereCols[colName] = val
		case "set":
			setCols[colName] = val
		default:
			return fmt.Errorf("invalid prefix %q (use where. or set.)", prefix)
		}
	}

	return g.db.Model(reflect.New(entityType).Interface()).Where(whereCols).Updates(setCols).Error
}

func (g *GormSteps) RegisterSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^all tables are truncated$`, func() error { return g.TruncateAll() })
	ctx.Step(`^table "([^"]*)" is truncated$`, g.TruncateTable)

	ctx.Step(`^"([^"]*)" table contains:$`, g.SeedTable)
	ctx.Step(`^I update the "([^"]*)" table with:$`, g.UpdateTable)
	ctx.Step(`^I delete from the "([^"]*)" table with:$`, g.DeleteFrom)

	ctx.Step(`^"([^"]*)" table should contain:$`, g.AssertContains)
	ctx.Step(`^"([^"]*)" table should not contain:$`, g.AssertNotContains)
	ctx.Step(`^there should be exactly (\d+) records? in the "([^"]*)" table$`, g.AssertCount)
	ctx.Step(`^there should be exactly (\d+) records? in the "([^"]*)" table with:$`, g.AssertCountWhere)
}

func (g *GormSteps) resolveType(table string) (reflect.Type, error) {
	if t, ok := g.opts.Registry[table]; ok {
		return t, nil
	}

	for _, model := range g.opts.Models {
		s, err := schema.Parse(&model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			continue
		}
		if s.Table == table {
			return reflect.TypeOf(model), nil
		}
	}

	return nil, fmt.Errorf("table %q not registered", table)
}

func (g *GormSteps) columnName(entityType reflect.Type, fieldName string) string {
	field, found := entityType.FieldByName(fieldName)
	if !found {
		return fieldName
	}

	if tag := field.Tag.Get("gorm"); tag != "" {
		for _, part := range strings.Split(tag, ";") {
			if col, ok := strings.CutPrefix(part, "column:"); ok {
				return col
			}
		}
	}

	if tag := field.Tag.Get("json"); tag != "" {
		name := strings.Split(tag, ",")[0]
		if name != "" && name != "-" {
			return name
		}
	}

	return fieldName
}

func (g *GormSteps) buildConditions(entityType reflect.Type, headers []string, row *messages.PickleTableRow) map[string]any {
	conditions := make(map[string]any)

	for i, cell := range row.Cells {
		if i >= len(headers) {
			break
		}

		fieldName := headers[i]
		colName := g.columnName(entityType, fieldName)

		field, found := entityType.FieldByName(fieldName)
		if !found {
			conditions[colName] = parseValue(cell.Value)
			continue
		}

		val, err := parseTypedValue(cell.Value, field.Type)
		if err != nil {
			conditions[colName] = parseValue(cell.Value)
			continue
		}

		conditions[colName] = val
	}

	return conditions
}

func (g *GormSteps) assertContainsRaw(table string, data *godog.Table) error {
	if len(data.Rows) < 2 {
		return fmt.Errorf("need header + data row")
	}

	headers := headerNames(data.Rows[0])

	for rowIdx, row := range data.Rows[1:] {
		query := g.db.Table(table)
		for i, cell := range row.Cells {
			if i >= len(headers) {
				break
			}
			val := parseValue(cell.Value)
			if val == nil {
				query = query.Where(fmt.Sprintf("%s IS NULL", headers[i]))
			} else {
				query = query.Where(fmt.Sprintf("%s = ?", headers[i]), val)
			}
		}

		var count int64
		query.Count(&count)
		if count == 0 {
			return fmt.Errorf("row %d not found in %s: %s", rowIdx+1, table, formatConditions(headers, row))
		}
	}

	return nil
}

func (g *GormSteps) allTableNames() []string {
	seen := make(map[string]bool)
	var tables []string

	for name := range g.opts.Registry {
		if !seen[name] {
			tables = append(tables, name)
			seen[name] = true
		}
	}

	for _, model := range g.opts.Models {
		s, err := schema.Parse(&model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			continue
		}
		if !seen[s.Table] {
			tables = append(tables, s.Table)
			seen[s.Table] = true
		}
	}

	return tables
}

func setFieldValue(field reflect.Value, value string) error {
	if !field.IsValid() || !field.CanSet() {
		return fmt.Errorf("field cannot be set")
	}

	if isNull(value) {
		return setNull(field)
	}

	if field.Kind() == reflect.Pointer {
		elem := reflect.New(field.Type().Elem()).Elem()
		if err := setFieldValue(elem, value); err != nil {
			return err
		}
		field.Set(elem.Addr())
		return nil
	}

	if field.Type() == reflect.TypeFor[uuid.UUID]() {
		id, err := uuid.Parse(value)
		if err != nil {
			return fmt.Errorf("parse UUID: %w", err)
		}
		field.Set(reflect.ValueOf(id))
		return nil
	}

	if field.Type() == reflect.TypeFor[time.Time]() {
		t, err := parseTime(value)
		if err != nil {
			return err
		}
		field.Set(reflect.ValueOf(t))
		return nil
	}

	if field.Type() == reflect.TypeFor[gorm.DeletedAt]() {
		t, err := parseTime(value)
		if err != nil {
			return err
		}
		field.Set(reflect.ValueOf(gorm.DeletedAt{Time: t, Valid: true}))
		return nil
	}

	if field.Type() == reflect.TypeFor[json.RawMessage]() {
		field.Set(reflect.ValueOf(json.RawMessage(value)))
		return nil
	}

	if field.Kind() == reflect.Struct || field.Kind() == reflect.Map || field.Kind() == reflect.Slice {
		target := reflect.New(field.Type()).Interface()
		if err := json.Unmarshal([]byte(value), target); err != nil {
			return fmt.Errorf("parse JSON for %s: %w", field.Type(), err)
		}
		field.Set(reflect.ValueOf(target).Elem())
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(f)
	default:
		return fmt.Errorf("unsupported type: %s", field.Kind())
	}

	return nil
}

func parseTypedValue(value string, targetType reflect.Type) (any, error) {
	tmp := reflect.New(targetType).Elem()
	if err := setFieldValue(tmp, value); err != nil {
		return nil, err
	}
	return tmp.Interface(), nil
}

func parseValue(s string) interface{} {
	if isNull(s) {
		return nil
	}
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	return s
}

func isNull(value string) bool {
	switch value {
	case "<nil>", "nil", "null", "NULL", "<null>":
		return true
	}
	return false
}

func setNull(field reflect.Value) error {
	switch field.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map:
		field.Set(reflect.Zero(field.Type()))
		return nil
	}
	return fmt.Errorf("cannot set null on non-nullable type %s", field.Type())
}

func parseTime(value string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999",
		"2006-01-02T15:04:05Z07:00",
	}

	for _, f := range formats {
		if t, err := time.Parse(f, value); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("cannot parse time %q", value)
}

func headerNames(row *messages.PickleTableRow) []string {
	names := make([]string, len(row.Cells))
	for i, cell := range row.Cells {
		names[i] = cell.Value
	}
	return names
}

func formatConditions(headers []string, row *messages.PickleTableRow) string {
	parts := make([]string, 0, len(row.Cells))
	for i, cell := range row.Cells {
		if i < len(headers) {
			parts = append(parts, fmt.Sprintf("%s=%s", headers[i], cell.Value))
		}
	}
	return strings.Join(parts, ", ")
}
