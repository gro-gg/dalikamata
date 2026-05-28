package query

// SortOrder specifies the direction of a sort.
type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

// SortField sorts results by the named field in the given direction.
type SortField struct {
	Field string    `json:"field"`
	Order SortOrder `json:"order"`
}
