package query

import (
	"fmt"
	"sort"
	"time"
)

// EvaluateAggs runs the aggregation tree against projected items and returns
// a result map keyed by aggregation name. items must already be filtered
// (EvaluateAggs does not re-apply query.Filter). Returns nil when aggs is empty.
func EvaluateAggs(items []map[string]any, aggs map[string]Aggregation) (map[string]AggregationResult, error) {
	if len(aggs) == 0 {
		return nil, nil
	}
	result := make(map[string]AggregationResult, len(aggs))
	for name, agg := range aggs {
		r, err := applyAgg(items, agg)
		if err != nil {
			return nil, fmt.Errorf("aggregation %q: %w", name, err)
		}
		result[name] = r
	}
	return result, nil
}

func applyAgg(items []map[string]any, agg Aggregation) (AggregationResult, error) {
	switch agg.Op {
	case AggTerms:
		return applyTerms(items, agg)
	case AggDateHistogram:
		return applyDateHistogram(items, agg)
	case AggHistogram:
		return applyHistogram(items, agg)
	default:
		return AggregationResult{}, fmt.Errorf("unknown aggregation op: %q", agg.Op)
	}
}

// applyTerms groups items by the string representation of a field value,
// preserving insertion order for within-group items and sorting buckets
// lexicographically for deterministic output.
//
// A []string field value (e.g. component_name/team_name/owner on enriched
// Workflow entities, which may resolve to several owners) fans the item out
// into one bucket per distinct element instead of a single bucket keyed by the
// whole slice — Elasticsearch-style multi-valued terms aggregation. An empty
// []string skips the item entirely (unlike a missing/nil scalar field, which
// still buckets under "<nil>"): there is no owner to attribute the item to.
func applyTerms(items []map[string]any, agg Aggregation) (AggregationResult, error) {
	type group struct {
		key   any
		items []map[string]any
	}
	var order []string
	groups := make(map[string]*group)

	addToBucket := func(key any, item map[string]any) {
		k := fmt.Sprint(key)
		if _, ok := groups[k]; !ok {
			groups[k] = &group{key: key}
			order = append(order, k)
		}
		groups[k].items = append(groups[k].items, item)
	}

	for _, item := range items {
		val := item[agg.Field]
		if list, isList := val.([]string); isList {
			seen := make(map[string]bool, len(list))
			for _, elem := range list {
				if seen[elem] {
					continue
				}
				seen[elem] = true
				addToBucket(elem, item)
			}
			continue
		}
		addToBucket(val, item)
	}

	sort.Strings(order)

	buckets := make([]Bucket, 0, len(order))
	for _, k := range order {
		g := groups[k]
		b := Bucket{
			Key:      g.key,
			DocCount: uint64(len(g.items)),
		}
		if len(agg.Aggs) > 0 {
			subAggs, err := EvaluateAggs(g.items, agg.Aggs)
			if err != nil {
				return AggregationResult{}, err
			}
			b.Aggs = subAggs
		}
		buckets = append(buckets, b)
	}
	return AggregationResult{Buckets: buckets}, nil
}

// applyDateHistogram groups time.Time field values into calendar-interval
// buckets, formatted using agg.Format (Go time layout). Buckets are sorted
// lexicographically — standard date formats like "2006-01" sort chronologically.
func applyDateHistogram(items []map[string]any, agg Aggregation) (AggregationResult, error) {
	layout := agg.Format
	if layout == "" {
		layout = time.RFC3339
	}

	type group struct {
		items []map[string]any
	}
	var order []string
	seen := make(map[string]bool)
	groups := make(map[string]*group)

	for _, item := range items {
		val, ok := item[agg.Field]
		if !ok {
			continue
		}
		t, ok := val.(time.Time)
		if !ok {
			return AggregationResult{}, fmt.Errorf("date_histogram field %q: got %T, want time.Time", agg.Field, val)
		}
		key := truncateTime(t, agg.Interval).Format(layout)
		if !seen[key] {
			seen[key] = true
			order = append(order, key)
			groups[key] = &group{}
		}
		groups[key].items = append(groups[key].items, item)
	}

	sort.Strings(order)

	buckets := make([]Bucket, 0, len(order))
	for _, k := range order {
		g := groups[k]
		b := Bucket{
			Key:      k,
			DocCount: uint64(len(g.items)),
		}
		if len(agg.Aggs) > 0 {
			subAggs, err := EvaluateAggs(g.items, agg.Aggs)
			if err != nil {
				return AggregationResult{}, err
			}
			b.Aggs = subAggs
		}
		buckets = append(buckets, b)
	}
	return AggregationResult{Buckets: buckets}, nil
}

// applyHistogram groups numeric field values into explicit-bound buckets with
// Prometheus-style cumulative semantics. Sub-aggregations are not supported on
// histogram leaves in v1.
func applyHistogram(items []map[string]any, agg Aggregation) (AggregationResult, error) {
	if len(agg.Buckets) == 0 {
		return AggregationResult{}, fmt.Errorf("histogram on field %q: no bucket boundaries defined", agg.Field)
	}
	if len(agg.Aggs) > 0 {
		return AggregationResult{}, fmt.Errorf("histogram: sub-aggregations are not supported in v1")
	}

	bounds := make([]float64, len(agg.Buckets))
	copy(bounds, agg.Buckets)
	sort.Float64s(bounds)

	cumulative := make(map[float64]uint64, len(bounds))
	for _, b := range bounds {
		cumulative[b] = 0
	}

	var totalCount uint64
	var sum float64

	for _, item := range items {
		val, ok := item[agg.Field]
		if !ok {
			continue
		}
		f, err := toFloat64(agg.Field, val)
		if err != nil {
			return AggregationResult{}, err
		}
		totalCount++
		sum += f
		for _, b := range bounds {
			if f <= b {
				cumulative[b]++
			}
		}
	}

	buckets := make([]Bucket, 0, len(bounds))
	for _, b := range bounds {
		buckets = append(buckets, Bucket{Key: b, DocCount: cumulative[b]})
	}
	return AggregationResult{DocCount: totalCount, Sum: sum, Buckets: buckets}, nil
}

func truncateTime(t time.Time, interval string) time.Time {
	switch interval {
	case "hour":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
	case "day":
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	default: // "month" or empty
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	}
}

func toFloat64(field string, val any) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("histogram field %q: got %T, want numeric", field, val)
	}
}
