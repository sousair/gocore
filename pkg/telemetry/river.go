package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const riverScope = "github.com/sousair/gocore/pkg/telemetry/river"

// traceparentKey is the job metadata key both River middlewares use to carry
// the W3C traceparent across the queue boundary.
const traceparentKey = "traceparent"

// insertMiddleware injects the enqueuing trace's traceparent into every
// inserted job's metadata (JSON object), merging with whatever keys are
// already there rather than overwriting them.
//
// Embeds river.MiddlewareDefaults (not the deprecated
// river.JobInsertMiddlewareDefaults) per the type's own deprecation notice in
// river v0.39.0.
type insertMiddleware struct{ river.MiddlewareDefaults }

func (*insertMiddleware) InsertMany(ctx context.Context, manyParams []*rivertype.JobInsertParams, doInner func(context.Context) ([]*rivertype.JobInsertResult, error)) ([]*rivertype.JobInsertResult, error) {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if tp := carrier[traceparentKey]; tp != "" {
		for _, p := range manyParams {
			meta := map[string]any{}
			if len(p.Metadata) > 0 {
				_ = json.Unmarshal(p.Metadata, &meta)
			}
			meta[traceparentKey] = tp
			if b, err := json.Marshal(meta); err == nil {
				p.Metadata = b
			}
		}
	}
	return doInner(ctx)
}

// workerMiddleware extracts the enqueuing trace's traceparent (if present)
// from job metadata and starts a root span for the work attempt linked into
// that trace, then logs "river.job_failed" on error. The error is always
// returned unchanged so River's retry/discard semantics are untouched.
type workerMiddleware struct{ river.MiddlewareDefaults }

func (*workerMiddleware) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	meta := map[string]any{}
	if len(job.Metadata) > 0 {
		_ = json.Unmarshal(job.Metadata, &meta)
	}
	if tp, _ := meta[traceparentKey].(string); tp != "" {
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{traceparentKey: tp})
	}

	tracer := otel.Tracer(riverScope)
	ctx, span := tracer.Start(ctx, "river.work "+job.Kind,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("river.kind", job.Kind),
			attribute.String("river.queue", job.Queue),
			attribute.Int("river.attempt", job.Attempt),
		))
	defer span.End()

	err := doInner(ctx)
	if err != nil {
		RecordError(span, err, err.Error())
		slog.ErrorContext(ctx, "river.job_failed", Err(err),
			slog.String("kind", job.Kind), slog.String("queue", job.Queue), slog.Int("attempt", job.Attempt))
	}
	return err
}

// RiverInsertMiddleware injects the enqueuing trace's traceparent into every
// inserted job's metadata (merged with any existing keys, never overwriting
// them).
func RiverInsertMiddleware() rivertype.JobInsertMiddleware { return &insertMiddleware{} }

// RiverWorkerMiddleware extracts the traceparent from a job's metadata,
// starts a root span linked to the enqueuing trace, and logs
// "river.job_failed" (with telemetry.Err) when the job errors. Errors are
// always returned unchanged so River's retry semantics are untouched.
func RiverWorkerMiddleware() rivertype.WorkerMiddleware { return &workerMiddleware{} }

// RiverMiddleware is the convenience entry point apps pass to
// river.Config.Middleware.
func RiverMiddleware() []rivertype.Middleware {
	return []rivertype.Middleware{RiverInsertMiddleware(), RiverWorkerMiddleware()}
}
