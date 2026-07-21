package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/sousair/gocore/pkg/telemetry"
)

func withPropagator(t *testing.T, prop propagation.TextMapPropagator) {
	t.Helper()
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(prop)
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })
}

func withTracerProvider(t *testing.T, tp trace.TracerProvider) {
	t.Helper()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
}

func TestRiverInsertMiddleware(t *testing.T) {
	t.Run("given active span/when InsertMany/then traceparent injected and existing metadata keys preserved", func(t *testing.T) {
		withPropagator(t, propagation.TraceContext{})
		tp := sdktrace.NewTracerProvider()
		ctx, span := tp.Tracer("test").Start(context.Background(), "enqueue")
		defer span.End()

		params := []*rivertype.JobInsertParams{
			{Kind: "send_email", Metadata: []byte(`{"foo":"bar"}`)},
			{Kind: "send_sms"},                          // nil metadata
			{Kind: "send_push", Metadata: []byte(`{}`)}, // empty object metadata
		}
		var captured []*rivertype.JobInsertParams
		doInner := func(ctx context.Context) ([]*rivertype.JobInsertResult, error) {
			captured = params
			return nil, nil
		}

		mw := telemetry.RiverInsertMiddleware()
		if _, err := mw.InsertMany(ctx, params, doInner); err != nil {
			t.Fatal(err)
		}
		if captured == nil {
			t.Fatal("want doInner invoked")
		}

		for _, p := range captured {
			var meta map[string]any
			if err := json.Unmarshal(p.Metadata, &meta); err != nil {
				t.Fatalf("kind %s: metadata not json: %s", p.Kind, p.Metadata)
			}
			if tp, _ := meta["traceparent"].(string); tp == "" {
				t.Fatalf("kind %s: want traceparent injected, got %s", p.Kind, p.Metadata)
			}
		}

		var meta0 map[string]any
		_ = json.Unmarshal(captured[0].Metadata, &meta0)
		if meta0["foo"] != "bar" {
			t.Fatalf("want existing metadata key preserved, got %v", meta0)
		}
	})

	t.Run("given no active span/when InsertMany/then doInner still invoked and no panic", func(t *testing.T) {
		withPropagator(t, propagation.TraceContext{})
		calls := 0
		mw := telemetry.RiverInsertMiddleware()
		_, err := mw.InsertMany(context.Background(), nil, func(ctx context.Context) ([]*rivertype.JobInsertResult, error) {
			calls++
			return nil, nil
		})
		if err != nil || calls != 1 {
			t.Fatalf("want doInner called once with no error, got calls=%d err=%v", calls, err)
		}
	})
}

func TestRiverWorkerMiddleware(t *testing.T) {
	t.Run("given job metadata carries a fixed traceparent/when Work/then ctx span shares that trace id", func(t *testing.T) {
		withPropagator(t, propagation.TraceContext{})
		rec := tracetest.NewSpanRecorder()
		withTracerProvider(t, sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))

		job := &rivertype.JobRow{
			Kind:     "send_email",
			Queue:    "default",
			Attempt:  1,
			Metadata: []byte(`{"traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01","other":"x"}`),
		}

		var gotTraceID string
		mw := telemetry.RiverWorkerMiddleware()
		if err := mw.Work(context.Background(), job, func(ctx context.Context) error {
			gotTraceID = trace.SpanContextFromContext(ctx).TraceID().String()
			return nil
		}); err != nil {
			t.Fatal(err)
		}

		if gotTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatalf("want linked trace id honored, got %q", gotTraceID)
		}
		if len(rec.Ended()) != 1 {
			t.Fatalf("want 1 ended span, got %d", len(rec.Ended()))
		}
	})

	t.Run("given no metadata/when Work/then doInner still invoked with no panic", func(t *testing.T) {
		withTracerProvider(t, sdktrace.NewTracerProvider())
		mw := telemetry.RiverWorkerMiddleware()
		job := &rivertype.JobRow{Kind: "x", Queue: "default"}
		calls := 0
		if err := mw.Work(context.Background(), job, func(ctx context.Context) error {
			calls++
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if calls != 1 {
			t.Fatalf("want doInner called once, got %d", calls)
		}
	})

	t.Run("given doInner errors/when Work/then error returned unchanged, span marked error, river.job_failed logged", func(t *testing.T) {
		rec := tracetest.NewSpanRecorder()
		withTracerProvider(t, sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))

		buf := &bytes.Buffer{}
		prevLog := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))
		t.Cleanup(func() { slog.SetDefault(prevLog) })

		job := &rivertype.JobRow{Kind: "flaky", Queue: "default", Attempt: 2}
		wantErr := errors.New("boom")
		mw := telemetry.RiverWorkerMiddleware()
		err := mw.Work(context.Background(), job, func(ctx context.Context) error {
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("want error returned unchanged, got %v", err)
		}

		spans := rec.Ended()
		if len(spans) != 1 || spans[0].Status().Code != codes.Error {
			t.Fatalf("want span marked error, got %+v", spans)
		}

		var logRec map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logRec); err != nil {
			t.Fatalf("log not json: %q", buf.String())
		}
		if logRec["msg"] != "river.job_failed" {
			t.Fatalf("want river.job_failed log, got %v", logRec)
		}
		if logRec["kind"] != "flaky" || logRec["queue"] != "default" || logRec["attempt"] != float64(2) {
			t.Fatalf("want kind/queue/attempt in log, got %v", logRec)
		}
		if logRec["error"] != "boom" {
			t.Fatalf("want error message in log, got %v", logRec["error"])
		}
	})

	t.Run("given a successful and a failing job/when Work/then river.job.duration histogram records both outcomes", func(t *testing.T) {
		withTracerProvider(t, sdktrace.NewTracerProvider())
		reader := withMeterProvider(t)

		mw := telemetry.RiverWorkerMiddleware()
		okJob := &rivertype.JobRow{Kind: "send_email", Queue: "default"}
		if err := mw.Work(context.Background(), okJob, func(ctx context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		failJob := &rivertype.JobRow{Kind: "send_email", Queue: "default"}
		wantErr := errors.New("boom")
		if err := mw.Work(context.Background(), failJob, func(ctx context.Context) error { return wantErr }); !errors.Is(err, wantErr) {
			t.Fatalf("want error returned unchanged, got %v", err)
		}

		points := histogramDataPoints(t, reader, "river.job.duration")
		if len(points) != 2 {
			t.Fatalf("want 2 histogram data points (one per outcome), got %d", len(points))
		}
		outcomes := map[string]bool{}
		for _, p := range points {
			if p.Count != 1 {
				t.Fatalf("want count 1 per data point, got %d", p.Count)
			}
			for _, a := range p.Attributes.ToSlice() {
				if a.Key == "river.outcome" {
					outcomes[a.Value.Emit()] = true
				}
			}
		}
		if !outcomes["success"] || !outcomes["error"] {
			t.Fatalf("want success and error outcomes recorded, got %v", outcomes)
		}
	})
}

func TestRiverMiddleware(t *testing.T) {
	t.Run("given RiverMiddleware/when constructed/then returns insert and worker middleware", func(t *testing.T) {
		mws := telemetry.RiverMiddleware()
		if len(mws) != 2 {
			t.Fatalf("want 2 middleware, got %d", len(mws))
		}
		if _, ok := mws[0].(rivertype.JobInsertMiddleware); !ok {
			t.Fatal("want first middleware to implement JobInsertMiddleware")
		}
		if _, ok := mws[1].(rivertype.WorkerMiddleware); !ok {
			t.Fatal("want second middleware to implement WorkerMiddleware")
		}
	})
}
