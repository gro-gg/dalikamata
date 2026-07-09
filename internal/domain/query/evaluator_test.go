package query_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
)

var (
	t0 = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 = time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t2 = time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
)

func fields(kvs ...any) map[string]any {
	m := make(map[string]any, len(kvs)/2)
	for i := 0; i < len(kvs)-1; i += 2 {
		m[kvs[i].(string)] = kvs[i+1]
	}
	return m
}

// ---- OpTerm ----------------------------------------------------------------

func TestMatchTerm(t *testing.T) {
	is := is.New(t)

	f := query.Filter{Op: query.OpTerm, Field: "state", Value: query.Ptr(query.StringValue("OPEN"))}

	ok, err := query.Match(&f, fields("state", "OPEN"))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields("state", "MERGED"))
	is.NoErr(err)
	is.True(!ok)

	// Missing field → no match, no error.
	ok, err = query.Match(&f, fields())
	is.NoErr(err)
	is.True(!ok)
}

func TestMatchTermTypeMismatch(t *testing.T) {
	is := is.New(t)
	f := query.Filter{Op: query.OpTerm, Field: "count", Value: query.Ptr(query.StringValue("5"))}
	// count is an int in the projection
	_, err := query.Match(&f, fields("count", 5))
	is.True(err != nil)
}

// ---- OpTerms ---------------------------------------------------------------

func TestMatchTerms(t *testing.T) {
	is := is.New(t)

	f := query.Filter{
		Op:     query.OpTerms,
		Field:  "state",
		Values: []query.Value{query.StringValue("MERGED"), query.StringValue("DECLINED")},
	}

	ok, err := query.Match(&f, fields("state", "MERGED"))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields("state", "OPEN"))
	is.NoErr(err)
	is.True(!ok)

	// Empty values list → never matches.
	empty := query.Filter{Op: query.OpTerms, Field: "state", Values: nil}
	ok, err = query.Match(&empty, fields("state", "MERGED"))
	is.NoErr(err)
	is.True(!ok)
}

// ---- OpRange (numeric) -----------------------------------------------------

func TestMatchRangeInt(t *testing.T) {
	is := is.New(t)

	between := query.Filter{
		Op:    query.OpRange,
		Field: "number",
		Range: &query.Range{
			GTE: query.Ptr(query.IntValue(10)),
			LTE: query.Ptr(query.IntValue(20)),
		},
	}

	for _, tt := range []struct {
		val  int64
		want bool
	}{
		{9, false},
		{10, true},
		{15, true},
		{20, true},
		{21, false},
	} {
		ok, err := query.Match(&between, fields("number", tt.val))
		is.NoErr(err)
		is.Equal(ok, tt.want)
	}
}

func TestMatchRangeGtLt(t *testing.T) {
	is := is.New(t)
	f := query.Filter{
		Op:    query.OpRange,
		Field: "duration",
		Range: &query.Range{
			GT: query.Ptr(query.FloatValue(0.0)),
			LT: query.Ptr(query.FloatValue(60.0)),
		},
	}

	ok, err := query.Match(&f, fields("duration", float64(0)))
	is.NoErr(err)
	is.True(!ok) // strictly greater than 0

	ok, err = query.Match(&f, fields("duration", float64(30)))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields("duration", float64(60)))
	is.NoErr(err)
	is.True(!ok) // strictly less than 60
}

// ---- OpRange (time) --------------------------------------------------------

func TestMatchRangeTime(t *testing.T) {
	is := is.New(t)
	f := query.Filter{
		Op:    query.OpRange,
		Field: "timestamp",
		Range: &query.Range{
			GTE: query.Ptr(query.TimeValue(t0)),
			LTE: query.Ptr(query.TimeValue(t2)),
		},
	}

	ok, err := query.Match(&f, fields("timestamp", t0))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields("timestamp", t1))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields("timestamp", t2.Add(time.Second)))
	is.NoErr(err)
	is.True(!ok)
}

// ---- OpExists --------------------------------------------------------------

func TestMatchExists(t *testing.T) {
	is := is.New(t)
	f := query.Filter{Op: query.OpExists, Field: "author"}

	ok, err := query.Match(&f, fields("author", "alice"))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields())
	is.NoErr(err)
	is.True(!ok)
}

// ---- []string (list-valued) fields -----------------------------------------

func TestMatchTermListField(t *testing.T) {
	is := is.New(t)
	f := query.Filter{Op: query.OpTerm, Field: "team_name", Value: query.Ptr(query.StringValue("backend-team"))}

	ok, err := query.Match(&f, fields("team_name", []string{"backend-team", "platform-team"}))
	is.NoErr(err)
	is.True(ok) // matches: at least one element equals the filter value

	ok, err = query.Match(&f, fields("team_name", []string{"platform-team"}))
	is.NoErr(err)
	is.True(!ok)

	ok, err = query.Match(&f, fields("team_name", []string{}))
	is.NoErr(err)
	is.True(!ok)
}

func TestMatchTermsListField(t *testing.T) {
	is := is.New(t)
	f := query.Filter{
		Op:     query.OpTerms,
		Field:  "team_name",
		Values: []query.Value{query.StringValue("backend-team"), query.StringValue("ui-team")},
	}

	ok, err := query.Match(&f, fields("team_name", []string{"platform-team", "ui-team"}))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields("team_name", []string{"platform-team"}))
	is.NoErr(err)
	is.True(!ok)
}

func TestMatchRangeListFieldErrors(t *testing.T) {
	is := is.New(t)
	f := query.Filter{
		Op:    query.OpRange,
		Field: "team_name",
		Range: &query.Range{GTE: query.Ptr(query.StringValue("a"))},
	}
	_, err := query.Match(&f, fields("team_name", []string{"backend-team"}))
	is.True(err != nil)
}

func TestMatchExistsListField(t *testing.T) {
	is := is.New(t)
	f := query.Filter{Op: query.OpExists, Field: "team_name"}

	ok, err := query.Match(&f, fields("team_name", []string{"backend-team"}))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields("team_name", []string{}))
	is.NoErr(err)
	is.True(!ok) // empty list: field present but has no values

	ok, err = query.Match(&f, fields())
	is.NoErr(err)
	is.True(!ok) // field absent entirely
}

// ---- OpBool ----------------------------------------------------------------

func TestMatchBoolMust(t *testing.T) {
	is := is.New(t)
	f := query.Filter{
		Op: query.OpBool,
		Must: []query.Filter{
			{Op: query.OpTerm, Field: "repo_id", Value: query.Ptr(query.StringValue("PROJ/repo"))},
			{Op: query.OpTerm, Field: "state", Value: query.Ptr(query.StringValue("MERGED"))},
		},
	}

	ok, err := query.Match(&f, fields("repo_id", "PROJ/repo", "state", "MERGED"))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields("repo_id", "PROJ/repo", "state", "OPEN"))
	is.NoErr(err)
	is.True(!ok)
}

func TestMatchBoolMustNot(t *testing.T) {
	is := is.New(t)
	f := query.Filter{
		Op: query.OpBool,
		MustNot: []query.Filter{
			{Op: query.OpTerm, Field: "state", Value: query.Ptr(query.StringValue("OPEN"))},
		},
	}

	ok, err := query.Match(&f, fields("state", "OPEN"))
	is.NoErr(err)
	is.True(!ok)

	ok, err = query.Match(&f, fields("state", "MERGED"))
	is.NoErr(err)
	is.True(ok)
}

func TestMatchBoolShouldOnly(t *testing.T) {
	is := is.New(t)
	f := query.Filter{
		Op: query.OpBool,
		Should: []query.Filter{
			{Op: query.OpTerm, Field: "state", Value: query.Ptr(query.StringValue("MERGED"))},
			{Op: query.OpTerm, Field: "state", Value: query.Ptr(query.StringValue("DECLINED"))},
		},
	}

	ok, err := query.Match(&f, fields("state", "MERGED"))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields("state", "OPEN"))
	is.NoErr(err)
	is.True(!ok)
}

func TestMatchBoolMustAndShouldIgnoresShould(t *testing.T) {
	is := is.New(t)
	// When Must is present, Should clauses are optional (no minimum_should_match).
	f := query.Filter{
		Op: query.OpBool,
		Must: []query.Filter{
			{Op: query.OpTerm, Field: "repo_id", Value: query.Ptr(query.StringValue("A/repo"))},
		},
		Should: []query.Filter{
			{Op: query.OpTerm, Field: "state", Value: query.Ptr(query.StringValue("MERGED"))},
		},
	}

	// Must matches, Should does not — still passes.
	ok, err := query.Match(&f, fields("repo_id", "A/repo", "state", "OPEN"))
	is.NoErr(err)
	is.True(ok)
}

func TestMatchBoolNested(t *testing.T) {
	is := is.New(t)
	// (state=MERGED) AND NOT (author=bot)
	f := query.Filter{
		Op: query.OpBool,
		Must: []query.Filter{
			{Op: query.OpTerm, Field: "state", Value: query.Ptr(query.StringValue("MERGED"))},
		},
		MustNot: []query.Filter{
			{Op: query.OpBool, Must: []query.Filter{
				{Op: query.OpTerm, Field: "author", Value: query.Ptr(query.StringValue("bot"))},
			}},
		},
	}

	ok, err := query.Match(&f, fields("state", "MERGED", "author", "alice"))
	is.NoErr(err)
	is.True(ok)

	ok, err = query.Match(&f, fields("state", "MERGED", "author", "bot"))
	is.NoErr(err)
	is.True(!ok)
}

// ---- Nil filter ------------------------------------------------------------

func TestMatchNilFilter(t *testing.T) {
	is := is.New(t)
	ok, err := query.Match(nil, fields("anything", "value"))
	is.NoErr(err)
	is.True(ok)
}

// ---- Sort ------------------------------------------------------------------

func TestLessAsc(t *testing.T) {
	is := is.New(t)
	sort := []query.SortField{{Field: "timestamp", Order: query.SortAsc}}
	a := fields("timestamp", t0)
	b := fields("timestamp", t2)
	is.True(query.Less(sort, a, b))
	is.True(!query.Less(sort, b, a))
	is.True(!query.Less(sort, a, a))
}

func TestLessDesc(t *testing.T) {
	is := is.New(t)
	sort := []query.SortField{{Field: "timestamp", Order: query.SortDesc}}
	a := fields("timestamp", t2)
	b := fields("timestamp", t0)
	is.True(query.Less(sort, a, b))
}

func TestLessMultiField(t *testing.T) {
	is := is.New(t)
	sortSpec := []query.SortField{
		{Field: "repo_id", Order: query.SortAsc},
		{Field: "timestamp", Order: query.SortDesc},
	}
	// Same repo_id → secondary sort by timestamp desc (newer first).
	a := fields("repo_id", "A", "timestamp", t2)
	b := fields("repo_id", "A", "timestamp", t0)
	is.True(query.Less(sortSpec, a, b))
}

// ---- Paginate --------------------------------------------------------------

func TestPaginate(t *testing.T) {
	is := is.New(t)
	items := []int{0, 1, 2, 3, 4}

	is.Equal(query.Paginate(items, 0, 3), []int{0, 1, 2})
	is.Equal(query.Paginate(items, 2, 3), []int{2, 3, 4})
	is.Equal(query.Paginate(items, 0, 0), []int{0, 1, 2, 3, 4}) // 0 = all
	is.Equal(len(query.Paginate(items, 10, 5)), 0)              // from out of range
	is.Equal(query.Paginate(items, 3, 100), []int{3, 4})        // size beyond end
}

// ---- JSON round-trip -------------------------------------------------------

func TestJSONRoundTrip(t *testing.T) {
	is := is.New(t)
	original := query.Query{
		Entity: query.EntityCommit,
		Filter: &query.Filter{
			Op: query.OpBool,
			Must: []query.Filter{
				{Op: query.OpTerm, Field: query.CommitRepoID, Value: query.Ptr(query.StringValue("PROJ/repo"))},
				{Op: query.OpRange, Field: query.CommitTimestamp, Range: &query.Range{
					GTE: query.Ptr(query.TimeValue(t0)),
					LT:  query.Ptr(query.TimeValue(t2)),
				}},
			},
		},
		Sort: []query.SortField{{Field: query.CommitTimestamp, Order: query.SortDesc}},
		From: 0,
		Size: 10,
	}

	b, err := json.Marshal(original)
	is.NoErr(err)

	var decoded query.Query
	is.NoErr(json.Unmarshal(b, &decoded))

	is.Equal(decoded.Entity, original.Entity)
	is.Equal(decoded.Sort, original.Sort)
	is.Equal(decoded.Size, original.Size)
	is.Equal(decoded.Filter.Op, original.Filter.Op)
	is.Equal(len(decoded.Filter.Must), 2)
	is.Equal(decoded.Filter.Must[0].Op, query.OpTerm)
	is.Equal(decoded.Filter.Must[1].Op, query.OpRange)
}
