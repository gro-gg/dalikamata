package query

// Entity identifies which domain entity a Query targets.
type Entity string

const (
	EntityRepo         Entity = "repo"
	EntityCommit       Entity = "commit"
	EntityPullRequest  Entity = "pullRequest"
	EntityWorkflow     Entity = "workflow"
	EntityWorkflowRun  Entity = "workflowRun"
	EntityWorkflowTask Entity = "workflowTask"
)

// Query is the top-level request type. It targets a single entity, applies an
// optional filter tree, sorts results, and paginates with from/size semantics.
// The "aggs" key is reserved for future aggregation support; passing it today
// has no effect.
type Query struct {
	Entity Entity      `json:"entity"`
	Filter *Filter     `json:"filter,omitempty"`
	Sort   []SortField `json:"sort,omitempty"`
	From   int         `json:"from,omitempty"`
	Size   int         `json:"size,omitempty"` // 0 = return all matches
}
