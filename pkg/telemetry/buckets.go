package telemetry

import sdkmetric "go.opentelemetry.io/otel/sdk/metric"

// durationBuckets are in seconds; the SDK default boundaries are ms-scaled.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// dbDurationView re-buckets otelpgx's db.client.operation.duration, whose
// instrument is created by a third party that sets no boundaries.
func dbDurationView() sdkmetric.View {
	return sdkmetric.NewView(
		sdkmetric.Instrument{Name: "db.client.operation.duration"},
		sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: durationBuckets}},
	)
}
