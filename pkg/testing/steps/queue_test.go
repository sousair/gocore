//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func redisAddr() string {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	return addr
}

func enqueueTask(t *testing.T, taskType string, payload map[string]interface{}, delay time.Duration) {
	t.Helper()
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr()})
	defer client.Close()

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	task := asynq.NewTask(taskType, data)
	_, err = client.Enqueue(task, asynq.ProcessIn(delay))
	require.NoError(t, err)
}

func TestQueueHelper_Flush(t *testing.T) {
	h := NewQueueHelper(redisAddr())
	defer h.Close()

	enqueueTask(t, "test:flush", map[string]interface{}{"id": "1"}, 10*time.Minute)

	err := h.AssertTaskCount("test:flush", 1)
	require.NoError(t, err)

	err = h.Flush()
	require.NoError(t, err)

	err = h.AssertQueueIsEmpty()
	require.NoError(t, err)
}

func TestQueueHelper_AssertTaskCount(t *testing.T) {
	h := NewQueueHelper(redisAddr())
	defer h.Close()
	require.NoError(t, h.Flush())

	enqueueTask(t, "test:count", map[string]interface{}{"id": "a"}, 10*time.Minute)
	enqueueTask(t, "test:count", map[string]interface{}{"id": "b"}, 10*time.Minute)
	enqueueTask(t, "test:other", map[string]interface{}{"id": "c"}, 10*time.Minute)

	err := h.AssertTaskCount("test:count", 2)
	assert.NoError(t, err)

	err = h.AssertTaskCount("test:other", 1)
	assert.NoError(t, err)

	err = h.AssertTaskCount("test:count", 5)
	assert.Error(t, err)

	require.NoError(t, h.Flush())
}

func TestQueueHelper_AssertHasTaskWithPayloadField(t *testing.T) {
	h := NewQueueHelper(redisAddr())
	defer h.Close()
	require.NoError(t, h.Flush())

	enqueueTask(t, "reminder:fire", map[string]interface{}{
		"reminder_id": "abc-123",
	}, 10*time.Minute)

	enqueueTask(t, "reminder:fire", map[string]interface{}{
		"reminder_id": "def-456",
	}, 10*time.Minute)

	err := h.AssertHasTaskWithPayloadField("reminder:fire", "reminder_id", "abc-123")
	assert.NoError(t, err)
	assert.Len(t, h.LastTasks, 1)

	err = h.AssertHasTaskWithPayloadField("reminder:fire", "reminder_id", "not-exist")
	assert.Error(t, err)

	require.NoError(t, h.Flush())
}

func TestQueueHelper_AssertNoTasks(t *testing.T) {
	h := NewQueueHelper(redisAddr())
	defer h.Close()
	require.NoError(t, h.Flush())

	err := h.AssertNoTasks("test:empty")
	assert.NoError(t, err)

	enqueueTask(t, "test:empty", map[string]interface{}{}, 10*time.Minute)

	err = h.AssertNoTasks("test:empty")
	assert.Error(t, err)

	require.NoError(t, h.Flush())
}

func TestQueueHelper_AssertQueueIsEmpty(t *testing.T) {
	h := NewQueueHelper(redisAddr())
	defer h.Close()
	require.NoError(t, h.Flush())

	err := h.AssertQueueIsEmpty()
	assert.NoError(t, err)

	enqueueTask(t, "test:notempty", map[string]interface{}{}, 10*time.Minute)

	err = h.AssertQueueIsEmpty()
	assert.Error(t, err)

	require.NoError(t, h.Flush())
}

func TestQueueHelper_CustomQueue(t *testing.T) {
	h := NewQueueHelper(redisAddr(), "critical")
	defer h.Close()
	require.NoError(t, h.Flush())

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr()})
	defer client.Close()

	data, _ := json.Marshal(map[string]interface{}{"id": "1"})
	_, err := client.Enqueue(asynq.NewTask("test:queue", data), asynq.ProcessIn(10*time.Minute), asynq.Queue("critical"))
	require.NoError(t, err)

	err = h.AssertTaskCount("test:queue", 1)
	assert.NoError(t, err)

	defaultH := NewQueueHelper(redisAddr())
	defer defaultH.Close()
	err = defaultH.AssertNoTasks("test:queue")
	assert.NoError(t, err)

	require.NoError(t, h.Flush())
}

func TestQueueHelper_PendingTasks(t *testing.T) {
	h := NewQueueHelper(redisAddr())
	defer h.Close()
	require.NoError(t, h.Flush())

	enqueueTask(t, "test:pending", map[string]interface{}{"id": "now"}, 0)

	err := h.AssertTaskCount("test:pending", 1)
	assert.NoError(t, err)

	require.NoError(t, h.Flush())
}

func TestQueueHelper_LastPayloads(t *testing.T) {
	h := NewQueueHelper(redisAddr())
	defer h.Close()
	require.NoError(t, h.Flush())

	enqueueTask(t, "test:payloads", map[string]interface{}{"user_id": "u1"}, 10*time.Minute)
	enqueueTask(t, "test:payloads", map[string]interface{}{"user_id": "u2"}, 10*time.Minute)

	err := h.AssertTaskCount("test:payloads", 2)
	require.NoError(t, err)

	assert.Len(t, h.LastPayloads, 2)

	var ids []string
	for _, raw := range h.LastPayloads {
		var p map[string]string
		require.NoError(t, json.Unmarshal(raw, &p))
		ids = append(ids, p["user_id"])
	}
	assert.Contains(t, ids, "u1")
	assert.Contains(t, ids, "u2")

	require.NoError(t, h.Flush())
}

var _ = context.Background
