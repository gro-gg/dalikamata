package query

// FilterOp is the discriminator for a Filter node.
type FilterOp string

const (
	// OpBool combines child filters via boolean logic (Must / MustNot / Should).
	OpBool FilterOp = "bool"
	// OpTerm matches a field's value exactly against a single Value.
	OpTerm FilterOp = "term"
	// OpTerms matches a field's value against any element in a Value list.
	OpTerms FilterOp = "terms"
	// OpRange matches a field's value within numeric or time bounds.
	OpRange FilterOp = "range"
	// OpExists matches documents where the named field is present.
	OpExists FilterOp = "exists"
)

// Filter is a single node in the filter tree.
// Exactly one of the compound (Must/MustNot/Should) or leaf (Field/Value/…)
// groups is populated according to Op.
type Filter struct {
	Op FilterOp `json:"op"`

	// Compound clauses (OpBool only).
	// Must: all child filters must match (AND).
	// MustNot: no child filter may match (NOT).
	// Should: when Must is empty at least one child must match (OR);
	//         when Must is non-empty Should clauses are optional.
	Must    []Filter `json:"must,omitempty"`
	MustNot []Filter `json:"must_not,omitempty"`
	Should  []Filter `json:"should,omitempty"`

	// Leaf fields (all ops except OpBool).
	Field  string  `json:"field,omitempty"`
	Value  *Value  `json:"value,omitempty"`  // OpTerm
	Values []Value `json:"values,omitempty"` // OpTerms
	Range  *Range  `json:"range,omitempty"`  // OpRange
}

// Range specifies numeric or time bounds for OpRange.
// At least one bound must be set; all four may be combined.
type Range struct {
	GT  *Value `json:"gt,omitempty"`
	GTE *Value `json:"gte,omitempty"`
	LT  *Value `json:"lt,omitempty"`
	LTE *Value `json:"lte,omitempty"`
}
