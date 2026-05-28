package query

// AggregationResult holds the output of one named aggregation.
//
// For bucket-style ops (terms, histogram, date_histogram) Buckets is populated.
// For histogram leaves, DocCount holds the total observation count and Sum holds
// the total of all observed values — both needed by prometheus.MustNewConstHistogram.
// Each histogram Bucket.DocCount is cumulative (observations ≤ upper bound).
type AggregationResult struct {
	Buckets  []Bucket `json:"buckets,omitempty"`
	DocCount uint64   `json:"doc_count,omitempty"`
	Sum      float64  `json:"sum,omitempty"`
}

// Bucket is one bin produced by a bucket-style aggregation.
// Key is always a JSON-safe type: string for terms and date_histogram,
// float64 for histogram upper bounds.
type Bucket struct {
	Key      any                          `json:"key"`
	DocCount uint64                       `json:"doc_count"`
	Aggs     map[string]AggregationResult `json:"aggs,omitempty"`
}
