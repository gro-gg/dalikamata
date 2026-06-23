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

	EntityTeam      Entity = "team"
	EntityComponent Entity = "component"
)

// Query is the top-level request type. It targets a single entity, applies an
// optional filter tree, sorts results, paginates with from/size semantics, and
// optionally requests server-side aggregations.
//
// When AggsOnly is true, entity hits are suppressed and only the aggregation
// result tree is returned (Size and Sort are ignored). When AggsOnly is false
// and Aggs is non-empty, both hits and aggregations are returned together.
type Query struct {
	Entity   Entity                 `json:"entity"`
	Filter   *Filter                `json:"filter,omitempty"`
	Sort     []SortField            `json:"sort,omitempty"`
	From     int                    `json:"from,omitempty"`
	Size     int                    `json:"size,omitempty"` // 0 = all matches
	AggsOnly bool                   `json:"aggs_only,omitempty"`
	Aggs     map[string]Aggregation `json:"aggs,omitempty"`
}
