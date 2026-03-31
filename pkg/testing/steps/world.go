//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

type World struct {
	ResponseData []byte
	LastID       string
	Vars         map[string]string
}

func NewWorld() *World {
	return &World{
		Vars: make(map[string]string),
	}
}

func (w *World) Reset() {
	w.ResponseData = nil
	w.LastID = ""
	w.Vars = make(map[string]string)
}

func (w *World) SetResponseData(data []byte) {
	w.ResponseData = data
}

func (w *World) ResponseJSON() (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(w.ResponseData, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return result, nil
}

func (w *World) ResponseField(field string) (string, error) {
	data, err := w.ResponseJSON()
	if err != nil {
		return "", err
	}
	val, ok := data[field]
	if !ok {
		return "", fmt.Errorf("field %q not found in response", field)
	}
	return fmt.Sprintf("%v", val), nil
}

func (w *World) iWaitForSeconds(ctx context.Context, seconds int) (context.Context, error) {
	time.Sleep(time.Duration(seconds) * time.Second)
	return ctx, nil
}

func (w *World) iSetVariable(key, value string) error {
	w.Vars[key] = value
	return nil
}

func (w *World) RegisterSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		w.Reset()
		return ctx, nil
	})

	ctx.Step(`^I wait for (\d+) seconds?$`, w.iWaitForSeconds)
	ctx.Step(`^I set "([^"]*)" to "([^"]*)"$`, w.iSetVariable)
}
