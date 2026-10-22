package telemetry_test

import (
	"context"
	"slices"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/sousair/gocore/pkg/telemetry"
)

func TestDBDurationViewSetsSecondBuckets(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithView(telemetry.DBDurationView()))
	hist, err := mp.Meter("test").Float64Histogram("db.client.operation.duration")
	if err != nil {
		t.Fatal(err)
	}
	hist.Record(context.Background(), 0.0004)

	points := histogramDataPoints(t, reader, "db.client.operation.duration")
	if len(points) != 1 {
		t.Fatalf("want 1 data point, got %d", len(points))
	}
	if !slices.Equal(points[0].Bounds, telemetry.DurationBuckets) {
		t.Fatalf("want bounds %v, got %v", telemetry.DurationBuckets, points[0].Bounds)
	}
	if points[0].BucketCounts[0] != 1 {
		t.Fatalf("want first bucket count 1, got %v", points[0].BucketCounts)
	}
}
