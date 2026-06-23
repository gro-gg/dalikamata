package query

// Ptr returns a pointer to v. Use it wherever a filter field requires *Value,
// eliminating the need for an intermediate variable:
//
//	query.Filter{Op: query.OpTerm, Field: query.PRState, Value: query.Ptr(query.StringValue("MERGED"))}
//
// Or via the constructor helpers below, which call Ptr internally:
//
//	query.TermFilter(query.PRState, query.StringValue("MERGED"))
func Ptr[T any](v T) *T { return &v }

// TermFilter returns a Filter matching documents where field equals value exactly.
func TermFilter(field string, value Value) Filter {
	return Filter{Op: OpTerm, Field: field, Value: Ptr(value)}
}

// TermsFilter returns a Filter matching documents where field equals any of values.
func TermsFilter(field string, values ...Value) Filter {
	return Filter{Op: OpTerms, Field: field, Values: values}
}

// RangeFilter returns a Filter matching documents where field falls within r.
// Use Ptr to set individual bounds:
//
//	query.RangeFilter(query.CommitTimestamp, query.Range{
//	    GTE: query.Ptr(query.TimeValue(from)),
//	    LTE: query.Ptr(query.TimeValue(to)),
//	})
func RangeFilter(field string, r Range) Filter {
	return Filter{Op: OpRange, Field: field, Range: &r}
}

// ExistsFilter returns a Filter matching documents where field is present.
func ExistsFilter(field string) Filter {
	return Filter{Op: OpExists, Field: field}
}

// AndFilter returns a bool Filter where all given filters must match (AND).
func AndFilter(filters ...Filter) Filter {
	return Filter{Op: OpBool, Must: filters}
}

// OrFilter returns a bool Filter where at least one of the given filters must match (OR).
// When Must clauses are also present, Should becomes an optional boost rather than required.
func OrFilter(filters ...Filter) Filter {
	return Filter{Op: OpBool, Should: filters}
}

// NotFilter returns a bool Filter where none of the given filters may match (NOT).
func NotFilter(filters ...Filter) Filter {
	return Filter{Op: OpBool, MustNot: filters}
}
