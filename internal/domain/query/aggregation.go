package query

// AggregationOp is the discriminator for an Aggregation node.
// Unknown ops are always errors — adapters must never silently drop results.
type AggregationOp string

const (
	// AggTerms groups items by distinct values of a field.
	AggTerms AggregationOp = "terms"
	// AggHistogram groups numeric values into explicit-bound buckets.
	// Bucket bounds are Prometheus-style: cumulative counts, explicit upper bounds.
	AggHistogram AggregationOp = "histogram"
	// AggDateHistogram groups time.Time values into calendar-interval buckets.
	AggDateHistogram AggregationOp = "date_histogram"
)

// Aggregation is a single node in the aggregation tree, mirroring the
// flat-discriminator style used by Filter. Sub-aggregations in Aggs are
// evaluated within each bucket produced by this node.
type Aggregation struct {
	Op    AggregationOp `json:"op"`
	Field string        `json:"field,omitempty"`

	// AggHistogram: explicit upper bounds (at least one required).
	Buckets []float64 `json:"buckets,omitempty"`

	// AggDateHistogram: calendar truncation interval and optional Go time layout.
	Interval string `json:"interval,omitempty"` // "month" | "day" | "hour"
	Format   string `json:"format,omitempty"`   // Go time layout, e.g. "2006-01"

	// Sub-aggregations applied within each bucket produced by this node.
	Aggs map[string]Aggregation `json:"aggs,omitempty"`
}
