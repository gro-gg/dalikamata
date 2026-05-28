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
// optional filter tree, sorts results, paginates with from/size semantics, and
// optionally requests server-side aggregations.
//
// Size -1 signals "aggregations only, no hits" — send to SubjectQueryAggregate
// to obtain only the aggregation result tree without streaming entity hits.
type Query struct {
	Entity Entity                 `json:"entity"`
	Filter *Filter                `json:"filter,omitempty"`
	Sort   []SortField            `json:"sort,omitempty"`
	From   int                    `json:"from,omitempty"`
	Size   int                    `json:"size,omitempty"` // 0 = all matches; -1 = aggregations only
	Aggs   map[string]Aggregation `json:"aggs,omitempty"`
}
