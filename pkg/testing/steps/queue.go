//go:build e2e

package steps

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"github.com/hibiken/asynq"
)

type QueueHelper struct {
	inspector *asynq.Inspector
	queue     string

	LastTasks    []*asynq.TaskInfo
	LastPayloads []json.RawMessage
}

func NewQueueHelper(redisAddr string, queue ...string) *QueueHelper {
	q := "default"
	if len(queue) > 0 && queue[0] != "" {
		q = queue[0]
	}

	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: redisAddr})
	return &QueueHelper{
		inspector: inspector,
		queue:     q,
	}
}

func (h *QueueHelper) Close() error {
	return h.inspector.Close()
}

func isQueueNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "does not exist")
}

func (h *QueueHelper) Flush() error {
	_, err1 := h.inspector.DeleteAllScheduledTasks(h.queue)
	_, err2 := h.inspector.DeleteAllPendingTasks(h.queue)
	_, err3 := h.inspector.DeleteAllArchivedTasks(h.queue)
	_, err4 := h.inspector.DeleteAllRetryTasks(h.queue)
	_, err5 := h.inspector.DeleteAllCompletedTasks(h.queue)

	for _, err := range []error{err1, err2, err3, err4, err5} {
		if err != nil && !isQueueNotFound(err) {
			return fmt.Errorf("flush queue %q: %w", h.queue, err)
		}
	}

	h.LastTasks = nil
	h.LastPayloads = nil
	return nil
}

func (h *QueueHelper) listAllByType(taskType string) ([]*asynq.TaskInfo, error) {
	pageSize := asynq.PageSize(200)
	var all []*asynq.TaskInfo

	scheduled, err := h.inspector.ListScheduledTasks(h.queue, pageSize)
	if err != nil && !isQueueNotFound(err) {
		return nil, fmt.Errorf("list scheduled: %w", err)
	}
	all = append(all, scheduled...)

	pending, err := h.inspector.ListPendingTasks(h.queue, pageSize)
	if err != nil && !isQueueNotFound(err) {
		return nil, fmt.Errorf("list pending: %w", err)
	}
	all = append(all, pending...)

	active, err := h.inspector.ListActiveTasks(h.queue, pageSize)
	if err != nil && !isQueueNotFound(err) {
		return nil, fmt.Errorf("list active: %w", err)
	}
	all = append(all, active...)

	if taskType == "" {
		return all, nil
	}

	var filtered []*asynq.TaskInfo
	for _, t := range all {
		if t.Type == taskType {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

func (h *QueueHelper) AssertTaskCount(taskType string, expected int) error {
	tasks, err := h.listAllByType(taskType)
	if err != nil {
		return err
	}

	h.LastTasks = tasks
	h.LastPayloads = nil
	for _, t := range tasks {
		h.LastPayloads = append(h.LastPayloads, t.Payload)
	}

	if len(tasks) != expected {
		return fmt.Errorf("expected %d tasks of type %q, got %d", expected, taskType, len(tasks))
	}
	return nil
}

func (h *QueueHelper) AssertHasTaskWithPayloadField(taskType, field, value string) error {
	tasks, err := h.listAllByType(taskType)
	if err != nil {
		return err
	}

	for _, t := range tasks {
		var payload map[string]interface{}
		if err := json.Unmarshal(t.Payload, &payload); err != nil {
			continue
		}
		if fmt.Sprintf("%v", payload[field]) == value {
			h.LastTasks = []*asynq.TaskInfo{t}
			h.LastPayloads = []json.RawMessage{t.Payload}
			return nil
		}
	}

	return fmt.Errorf("no task of type %q with %s=%q found in queue", taskType, field, value)
}

func (h *QueueHelper) AssertNoTasks(taskType string) error {
	return h.AssertTaskCount(taskType, 0)
}

func (h *QueueHelper) AssertQueueIsEmpty() error {
	tasks, err := h.listAllByType("")
	if err != nil {
		return err
	}
	if len(tasks) > 0 {
		types := make(map[string]int)
		for _, t := range tasks {
			types[t.Type]++
		}
		return fmt.Errorf("expected empty queue, got %d tasks: %v", len(tasks), types)
	}
	return nil
}

func (h *QueueHelper) RegisterSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^the queue is flushed$`, h.Flush)
	ctx.Step(`^the queue should be empty$`, h.AssertQueueIsEmpty)
	ctx.Step(`^there should be (\d+) "([^"]*)" tasks? in the queue$`, func(count int, taskType string) error {
		return h.AssertTaskCount(taskType, count)
	})
	ctx.Step(`^there should be no "([^"]*)" tasks in the queue$`, h.AssertNoTasks)
	ctx.Step(`^there should be a "([^"]*)" task with "([^"]*)" = "([^"]*)" in the queue$`, h.AssertHasTaskWithPayloadField)
}
