package httpx

import (
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/locales/en"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/sousair/gocore/pkg/telemetry"

	ut "github.com/go-playground/universal-translator"
	en_translations "github.com/go-playground/validator/v10/translations/en"
)

var (
	once       sync.Once
	v          *validator.Validate
	translator ut.Translator
)

func GetValidator() *validator.Validate {
	if v != nil {
		return v
	}

	once.Do(func() {
		v = validator.New()

		enT := en.New()
		uni := ut.New(enT, enT)

		var ok bool
		translator, ok = uni.GetTranslator("en")
		if ok {
			if err := en_translations.RegisterDefaultTranslations(v, translator); err != nil {
				slog.Warn("httpx.validator_translations_failed", telemetry.Err(err)) //nolint:sloglint // no ctx at construction
			}
		} else {
			panic("httpx: failed to get translator for 'en'")
		}
	})

	return v
}

func NewRequest[T any](e echo.Context) (*T, error) {
	reqValidator := GetValidator()
	var req T

	if err := e.Bind(&req); err != nil {
		return nil, ErrBadRequest
	}

	val := reflect.ValueOf(req)

	var err error
	switch val.Kind() {
	case reflect.Slice:
		for i := range val.Len() {
			item := val.Index(i).Interface()
			err = reqValidator.Struct(item)
		}
	default:
		err = reqValidator.Struct(req)
	}

	if err != nil {
		validationErrs, ok := err.(validator.ValidationErrors)
		if !ok {
			return nil, ErrInvalidRequest
		}

		translated := validationErrs.Translate(translator)

		var messages []string
		for _, msg := range translated {
			messages = append(messages, msg)
		}

		return nil, errors.New(strings.Join(messages, "; "))
	}

	return &req, nil
}
