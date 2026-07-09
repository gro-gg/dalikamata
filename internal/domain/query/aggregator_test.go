package query_test

import (
	"testing"
	"time"

	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
)

// ---- EvaluateAggs: empty aggs ----------------------------------------------

func TestEvaluateAggs_Empty(t *testing.T) {
	is := is.New(t)
	items := []map[string]any{{"x": "a"}}
	result, err := query.EvaluateAggs(items, nil)
	is.NoErr(err)
	is.True(result == nil)
}

// ---- terms -----------------------------------------------------------------

func TestAggTerms_Basic(t *testing.T) {
	is := is.New(t)
	items := []map[string]any{
		{"state": "OPEN"},
		{"state": "MERGED"},
		{"state": "OPEN"},
		{"state": "DECLINED"},
		{"state": "OPEN"},
	}
	aggs := map[string]query.Aggregation{
		"by_state": {Op: query.AggTerms, Field: "state"},
	}
	result, err := query.EvaluateAggs(items, aggs)
	is.NoErr(err)
	byState := result["by_state"]
	is.Equal(len(byState.Buckets), 3)

	// buckets are sorted lexicographically
	is.Equal(byState.Buckets[0].Key, "DECLINED")
	is.Equal(byState.Buckets[0].DocCount, uint64(1))
	is.Equal(byState.Buckets[1].Key, "MERGED")
	is.Equal(byState.Buckets[1].DocCount, uint64(1))
	is.Equal(byState.Buckets[2].Key, "OPEN")
	is.Equal(byState.Buckets[2].DocCount, uint64(3))
}

func TestAggTerms_MissingField(t *testing.T) {
	is := is.New(t)
	// items without the field are grouped under the empty string key
	items := []map[string]any{
		{"state": "OPEN"},
		{},
	}
	aggs := map[string]query.Aggregation{
		"by_state": {Op: query.AggTerms, Field: "state"},
	}
	result, err := query.EvaluateAggs(items, aggs)
	is.NoErr(err)
	is.Equal(len(result["by_state"].Buckets), 2)
}

// ---- terms: []string fan-out ------------------------------------------------

func TestAggTerms_ListFieldFansOut(t *testing.T) {
	is := is.New(t)
	items := []map[string]any{
		{"id": 1, "team_name": []string{"backend-team", "platform-team"}},
		{"id": 2, "team_name": []string{"platform-team"}},
	}
	aggs := map[string]query.Aggregation{
		"by_team": {Op: query.AggTerms, Field: "team_name"},
	}
	result, err := query.EvaluateAggs(items, aggs)
	is.NoErr(err)
	byTeam := result["by_team"]
	is.Equal(len(byTeam.Buckets), 2)
	is.Equal(byTeam.Buckets[0].Key, "backend-team")
	is.Equal(byTeam.Buckets[0].DocCount, uint64(1)) // item 1 only
	is.Equal(byTeam.Buckets[1].Key, "platform-team")
	is.Equal(byTeam.Buckets[1].DocCount, uint64(2)) // items 1 and 2
}

func TestAggTerms_ListFieldDedupesWithinItem(t *testing.T) {
	is := is.New(t)
	// A duplicate element in one item's list must not double-count that item.
	items := []map[string]any{
		{"team_name": []string{"backend-team", "backend-team"}},
	}
	aggs := map[string]query.Aggregation{
		"by_team": {Op: query.AggTerms, Field: "team_name"},
	}
	result, err := query.EvaluateAggs(items, aggs)
	is.NoErr(err)
	byTeam := result["by_team"]
	is.Equal(len(byTeam.Buckets), 1)
	is.Equal(byTeam.Buckets[0].DocCount, uint64(1))
}

func TestAggTerms_EmptyListFieldSkipsItem(t *testing.T) {
	is := is.New(t)
	// Unlike a missing/nil scalar (bucketed under "<nil>"), an empty []string
	// has no owner to attribute the item to and is skipped entirely.
	items := []map[string]any{
		{"team_name": []string{"backend-team"}},
		{"team_name": []string{}},
	}
	aggs := map[string]query.Aggregation{
		"by_team": {Op: query.AggTerms, Field: "team_name"},
	}
	result, err := query.EvaluateAggs(items, aggs)
	is.NoErr(err)
	byTeam := result["by_team"]
	is.Equal(len(byTeam.Buckets), 1)
	is.Equal(byTeam.Buckets[0].Key, "backend-team")
	is.Equal(byTeam.Buckets[0].DocCount, uint64(1))
}

// TestAggTerms_CorrelatedOwnerKeyAvoidsCrossProduct is a regression test for
// the aggregation-pairing bug that motivates the combined "team|component"
// owner field (see query.OwnerKey): nesting independently-fanned
// team_name/component_name terms aggregations would place an item owned by
// (alpha, svc-a) under every (team, component) combination reachable from its
// separately-fanned fields, including pairs the item does not actually have.
// Aggregating on the correlated key instead keeps each item's owner pairs intact.
func TestAggTerms_CorrelatedOwnerKeyAvoidsCrossProduct(t *testing.T) {
	is := is.New(t)
	items := []map[string]any{
		// Owned by two pairs: (alpha, svc-a) and (gamma, svc-c).
		{"owner": []string{query.OwnerKey("alpha", "svc-a"), query.OwnerKey("gamma", "svc-c")}, "secs": 10.0},
	}
	aggs := map[string]query.Aggregation{
		"by_owner": {
			Op:    query.AggTerms,
			Field: "owner",
			Aggs: map[string]query.Aggregation{
				"total": {Op: query.AggHistogram, Field: "secs", Buckets: []float64{100}},
			},
		},
	}
	result, err := query.EvaluateAggs(items, aggs)
	is.NoErr(err)
	byOwner := result["by_owner"]
	is.Equal(len(byOwner.Buckets), 2) // exactly the two actual pairs, not the 2x2 cross product

	seen := map[string]bool{}
	for _, b := range byOwner.Buckets {
		key := b.Key.(string)
		seen[key] = true
		is.Equal(b.Aggs["total"].DocCount, uint64(1))
	}
	is.True(seen[query.OwnerKey("alpha", "svc-a")])
	is.True(seen[query.OwnerKey("gamma", "svc-c")])
	is.True(!seen[query.OwnerKey("alpha", "svc-c")]) // cross-paired — must not appear
	is.True(!seen[query.OwnerKey("gamma", "svc-a")]) // cross-paired — must not appear
}

// ---- date_histogram --------------------------------------------------------

func TestAggDateHistogram_Month(t *testing.T) {
	is := is.New(t)
	jan := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2024, 2, 3, 0, 0, 0, 0, time.UTC)
	items := []map[string]any{
		{"created_at": jan},
		{"created_at": jan},
		{"created_at": feb},
	}
	aggs := map[string]query.Aggregation{
		"by_month": {Op: query.AggDateHistogram, Field: "created_at", Interval: "month", Format: "2006-01"},
	}
	result, err := query.EvaluateAggs(items, aggs)
	is.NoErr(err)
	byMonth := result["by_month"]
	is.Equal(len(byMonth.Buckets), 2)
	is.Equal(byMonth.Buckets[0].Key, "2024-01")
	is.Equal(byMonth.Buckets[0].DocCount, uint64(2))
	is.Equal(byMonth.Buckets[1].Key, "2024-02")
	is.Equal(byMonth.Buckets[1].DocCount, uint64(1))
}

func TestAggDateHistogram_WrongFieldType(t *testing.T) {
	is := is.New(t)
	items := []map[string]any{{"ts": "not-a-time"}}
	aggs := map[string]query.Aggregation{
		"bad": {Op: query.AggDateHistogram, Field: "ts", Interval: "month"},
	}
	_, err := query.EvaluateAggs(items, aggs)
	is.True(err != nil)
}

// ---- histogram -------------------------------------------------------------

func TestAggHistogram_Basic(t *testing.T) {
	is := is.New(t)
	// 3 observations: 1800s, 7200s, 90000s
	// buckets: [3600, 14400, 86400, 259200, 604800]
	// cumulative:  1     2      2      3       3
	items := []map[string]any{
		{"secs": 1800.0},
		{"secs": 7200.0},
		{"secs": 90000.0},
	}
	aggs := map[string]query.Aggregation{
		"hist": {Op: query.AggHistogram, Field: "secs", Buckets: []float64{3600, 14400, 86400, 259200, 604800}},
	}
	result, err := query.EvaluateAggs(items, aggs)
	is.NoErr(err)
	hist := result["hist"]
	is.Equal(hist.DocCount, uint64(3))
	is.Equal(hist.Sum, 1800.0+7200.0+90000.0)
	is.Equal(len(hist.Buckets), 5)
	is.Equal(hist.Buckets[0].Key, 3600.0)
	is.Equal(hist.Buckets[0].DocCount, uint64(1)) // 1800 ≤ 3600
	is.Equal(hist.Buckets[1].Key, 14400.0)
	is.Equal(hist.Buckets[1].DocCount, uint64(2)) // 7200 ≤ 14400
	is.Equal(hist.Buckets[2].Key, 86400.0)
	is.Equal(hist.Buckets[2].DocCount, uint64(2))
	is.Equal(hist.Buckets[3].Key, 259200.0)
	is.Equal(hist.Buckets[3].DocCount, uint64(3))
	is.Equal(hist.Buckets[4].Key, 604800.0)
	is.Equal(hist.Buckets[4].DocCount, uint64(3))
}

func TestAggHistogram_NoBuckets(t *testing.T) {
	is := is.New(t)
	aggs := map[string]query.Aggregation{
		"bad": {Op: query.AggHistogram, Field: "secs"},
	}
	_, err := query.EvaluateAggs([]map[string]any{}, aggs)
	is.True(err != nil)
}

func TestAggHistogram_SubAggsRejected(t *testing.T) {
	is := is.New(t)
	aggs := map[string]query.Aggregation{
		"bad": {Op: query.AggHistogram, Field: "secs", Buckets: []float64{100},
			Aggs: map[string]query.Aggregation{"inner": {Op: query.AggTerms, Field: "x"}}},
	}
	_, err := query.EvaluateAggs([]map[string]any{}, aggs)
	is.True(err != nil)
}

func TestAggHistogram_MissingField(t *testing.T) {
	is := is.New(t)
	// items without the field are skipped
	items := []map[string]any{{}, {}}
	aggs := map[string]query.Aggregation{
		"hist": {Op: query.AggHistogram, Field: "secs", Buckets: []float64{100}},
	}
	result, err := query.EvaluateAggs(items, aggs)
	is.NoErr(err)
	is.Equal(result["hist"].DocCount, uint64(0))
}

// ---- unknown op ------------------------------------------------------------

func TestAggUnknownOp(t *testing.T) {
	is := is.New(t)
	aggs := map[string]query.Aggregation{
		"bad": {Op: "percentiles", Field: "x"},
	}
	_, err := query.EvaluateAggs([]map[string]any{}, aggs)
	is.True(err != nil)
}

// ---- nested: terms → histogram ---------------------------------------------

func TestAggNested_TermsThenHistogram(t *testing.T) {
	is := is.New(t)
	items := []map[string]any{
		{"state": "OPEN", "secs": 1000.0},
		{"state": "OPEN", "secs": 5000.0},
		{"state": "MERGED", "secs": 50000.0},
	}
	aggs := map[string]query.Aggregation{
		"by_state": {
			Op:    query.AggTerms,
			Field: "state",
			Aggs: map[string]query.Aggregation{
				"cycle": {Op: query.AggHistogram, Field: "secs", Buckets: []float64{3600, 14400, 86400}},
			},
		},
	}
	result, err := query.EvaluateAggs(items, aggs)
	is.NoErr(err)

	byState := result["by_state"]
	// MERGED bucket
	var mergedBucket, openBucket query.Bucket
	for _, b := range byState.Buckets {
		if b.Key.(string) == "MERGED" {
			mergedBucket = b
		} else {
			openBucket = b
		}
	}

	mergedCycle := mergedBucket.Aggs["cycle"]
	is.Equal(mergedCycle.DocCount, uint64(1))
	is.Equal(mergedCycle.Sum, 50000.0)

	openCycle := openBucket.Aggs["cycle"]
	is.Equal(openCycle.DocCount, uint64(2))
	is.Equal(openCycle.Sum, 6000.0)
	// 1000 ≤ 3600 → cumulative[3600]=1; 5000 ≤ 14400 → cumulative[14400]=2
	is.Equal(openCycle.Buckets[0].DocCount, uint64(1))
	is.Equal(openCycle.Buckets[1].DocCount, uint64(2))
}
