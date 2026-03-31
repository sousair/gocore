//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/cucumber/godog"
)

type JSONSteps struct {
	world *World
}

func NewJSONSteps(world *World) *JSONSteps {
	return &JSONSteps{world: world}
}

func (j *JSONSteps) data() ([]byte, error) {
	if j.world == nil || j.world.ResponseData == nil {
		return nil, fmt.Errorf("no response data")
	}
	return j.world.ResponseData, nil
}

func (j *JSONSteps) isValid(_ context.Context) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func (j *JSONSteps) equals(_ context.Context, expected *godog.DocString) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	return jsonDeepEqual([]byte(expected.Content), data, "")
}

func (j *JSONSteps) notEquals(_ context.Context, expected *godog.DocString) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	if err := jsonDeepEqual([]byte(expected.Content), data, ""); err == nil {
		return fmt.Errorf("expected JSON to NOT equal the given value")
	}
	return nil
}

func (j *JSONSteps) pathEquals(_ context.Context, path, expected string) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	actual, err := extractPath(data, path)
	if err != nil {
		return err
	}
	actualStr := fmt.Sprintf("%v", actual)
	if actualStr != expected {
		return fmt.Errorf("path %q: expected %q, got %q", path, expected, actualStr)
	}
	return nil
}

func (j *JSONSteps) pathNotEquals(_ context.Context, path, expected string) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	actual, err := extractPath(data, path)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%v", actual) == expected {
		return fmt.Errorf("path %q: expected NOT %q", path, expected)
	}
	return nil
}

func (j *JSONSteps) pathExists(_ context.Context, path string) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	_, err = extractPath(data, path)
	return err
}

func (j *JSONSteps) pathNotExists(_ context.Context, path string) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	if _, err := extractPath(data, path); err == nil {
		return fmt.Errorf("path %q unexpectedly exists", path)
	}
	return nil
}

func (j *JSONSteps) pathIsNil(_ context.Context, path string) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	val, err := extractPath(data, path)
	if err != nil {
		return err
	}
	if val != nil {
		return fmt.Errorf("path %q: expected nil, got %v", path, val)
	}
	return nil
}

func (j *JSONSteps) pathIsNotNil(_ context.Context, path string) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	val, err := extractPath(data, path)
	if err != nil {
		return err
	}
	if val == nil {
		return fmt.Errorf("path %q: expected non-nil", path)
	}
	return nil
}

func (j *JSONSteps) pathIsEmpty(_ context.Context, path string) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	val, err := extractPath(data, path)
	if err != nil {
		return err
	}
	if val == nil {
		return nil
	}

	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.String:
		if rv.Len() > 0 {
			return fmt.Errorf("path %q: expected empty string, got %q", path, val)
		}
	case reflect.Slice, reflect.Map:
		if rv.Len() > 0 {
			return fmt.Errorf("path %q: expected empty, got %d elements", path, rv.Len())
		}
	default:
		return fmt.Errorf("path %q: expected empty, got %v", path, val)
	}
	return nil
}

func (j *JSONSteps) pathIsNotEmpty(_ context.Context, path string) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	val, err := extractPath(data, path)
	if err != nil {
		return err
	}
	if val == nil {
		return fmt.Errorf("path %q: is nil", path)
	}

	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Map:
		if rv.Len() == 0 {
			return fmt.Errorf("path %q: is empty", path)
		}
	}
	return nil
}

func (j *JSONSteps) pathMatchesRegex(_ context.Context, path, pattern string) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	val, err := extractPath(data, path)
	if err != nil {
		return err
	}
	matched, err := regexp.MatchString(pattern, fmt.Sprintf("%v", val))
	if err != nil {
		return fmt.Errorf("invalid regex: %w", err)
	}
	if !matched {
		return fmt.Errorf("path %q value %v does not match %q", path, val, pattern)
	}
	return nil
}

func (j *JSONSteps) pathIsType(_ context.Context, path, expectedType string) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	val, err := extractPath(data, path)
	if err != nil {
		return err
	}

	actualType := jsonType(val)
	if actualType != expectedType {
		return fmt.Errorf("path %q: expected type %q, got %q", path, expectedType, actualType)
	}
	return nil
}

func (j *JSONSteps) containsSubset(_ context.Context, expected *godog.DocString) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	return jsonContainsSubset(data, []byte(expected.Content))
}

func (j *JSONSteps) pathContainsSubset(_ context.Context, path string, expected *godog.DocString) error {
	data, err := j.data()
	if err != nil {
		return err
	}
	val, err := extractPathRaw(data, path)
	if err != nil {
		return err
	}
	return jsonContainsSubset(val, []byte(expected.Content))
}

func (j *JSONSteps) RegisterSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^(?:the )?JSON response is valid$`, j.isValid)
	ctx.Step(`^(?:the )?JSON response equals:$`, j.equals)
	ctx.Step(`^(?:the )?JSON response does not equal:$`, j.notEquals)

	ctx.Step(`^(?:the )?JSON response path "([^"]+)" equals "([^"]*)"$`, j.pathEquals)
	ctx.Step(`^(?:the )?JSON response path "([^"]+)" does not equal "([^"]*)"$`, j.pathNotEquals)
	ctx.Step(`^(?:the )?JSON response contains path "([^"]+)"$`, j.pathExists)
	ctx.Step(`^(?:the )?JSON response does not contain path "([^"]+)"$`, j.pathNotExists)

	ctx.Step(`^(?:the )?JSON response path "([^"]+)" is nil$`, j.pathIsNil)
	ctx.Step(`^(?:the )?JSON response path "([^"]+)" is not nil$`, j.pathIsNotNil)
	ctx.Step(`^(?:the )?JSON response path "([^"]+)" is empty$`, j.pathIsEmpty)
	ctx.Step(`^(?:the )?JSON response path "([^"]+)" is not empty$`, j.pathIsNotEmpty)

	ctx.Step(`^(?:the )?JSON response path "([^"]+)" matches regex "([^"]+)"$`, j.pathMatchesRegex)
	ctx.Step(`^(?:the )?JSON response path "([^"]+)" is of type "([^"]+)"$`, j.pathIsType)

	ctx.Step(`^(?:the )?JSON response contains the values:$`, j.containsSubset)
	ctx.Step(`^(?:the )?JSON response path "([^"]+)" contains the values:$`, j.pathContainsSubset)
}

func extractPath(data []byte, path string) (interface{}, error) {
	var root interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	parts := strings.Split(path, ".")
	current := root

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("path %q: key %q not found", path, part)
			}
			current = val
		case []interface{}:
			idx := 0
			if _, err := fmt.Sscanf(part, "%d", &idx); err != nil {
				return nil, fmt.Errorf("path %q: %q is not an array index", path, part)
			}
			if idx < 0 || idx >= len(v) {
				return nil, fmt.Errorf("path %q: index %d out of range (len %d)", path, idx, len(v))
			}
			current = v[idx]
		default:
			return nil, fmt.Errorf("path %q: cannot traverse into %T at %q", path, current, part)
		}
	}

	return current, nil
}

func extractPathRaw(data []byte, path string) ([]byte, error) {
	val, err := extractPath(data, path)
	if err != nil {
		return nil, err
	}
	return json.Marshal(val)
}

func jsonType(val interface{}) string {
	if val == nil {
		return "null"
	}
	switch val.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return fmt.Sprintf("%T", val)
	}
}

func jsonDeepEqual(expected, actual []byte, path string) error {
	var exp, act interface{}

	if err := json.Unmarshal(expected, &exp); err != nil {
		return fmt.Errorf("parse expected JSON: %w", err)
	}

	if err := json.Unmarshal(actual, &act); err != nil {
		return fmt.Errorf("parse actual JSON: %w", err)
	}

	if path != "" {
		val, err := extractPath(actual, path)
		if err != nil {
			return err
		}
		act = val
	}

	if !reflect.DeepEqual(exp, act) {
		expJSON, _ := json.MarshalIndent(exp, "", "  ")
		actJSON, _ := json.MarshalIndent(act, "", "  ")
		return fmt.Errorf("JSON mismatch:\nexpected:\n%s\nactual:\n%s", expJSON, actJSON)
	}
	return nil
}

func jsonContainsSubset(actual, expected []byte) error {
	var actMap, expMap map[string]interface{}

	if err := json.Unmarshal(actual, &actMap); err != nil {
		return fmt.Errorf("parse actual: %w", err)
	}
	if err := json.Unmarshal(expected, &expMap); err != nil {
		return fmt.Errorf("parse expected: %w", err)
	}

	for key, expVal := range expMap {
		actVal, ok := actMap[key]
		if !ok {
			return fmt.Errorf("key %q missing from response", key)
		}
		if !reflect.DeepEqual(expVal, actVal) {
			return fmt.Errorf("key %q: expected %v, got %v", key, expVal, actVal)
		}
	}

	return nil
}
