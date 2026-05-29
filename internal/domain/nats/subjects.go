package nats

// Ingest stream configuration.
const (
	StreamIngest     = "ingest.>"
	StreamIngestName = "INGEST"
)

// Ingest subjects — events published by crawlers and consumed by the domain.
const (
	SubjectRepo        = "ingest.git.repo"
	SubjectCommit      = "ingest.git.commit"
	SubjectPullRequest = "ingest.git.pullrequest"

	SubjectCicdWorkflow     = "ingest.cicd.workflow"
	SubjectCicdWorkflowRun  = "ingest.cicd.workflowRun"
	SubjectCicdWorkflowTask = "ingest.cicd.workflowTask"

	SubjectPlatformTeam      = "ingest.platform.team"
	SubjectPlatformComponent = "ingest.platform.component"
)

// Query subjects — request-reply subjects for reading domain entities.
// Each subject mirrors the ingest hierarchy under the "query." prefix.
const (
	SubjectQueryRepo            = "query.git.repo"
	SubjectQueryCommit          = "query.git.commit"
	SubjectQueryPullRequest     = "query.git.pullrequest"
	SubjectQueryCicdWorkflow    = "query.cicd.workflow"
	SubjectQueryCicdWorkflowRun = "query.cicd.workflowRun"
	SubjectQueryCicdTask        = "query.cicd.workflowTask"

	SubjectQueryPlatformTeam      = "query.platform.team"
	SubjectQueryPlatformComponent = "query.platform.component"
)

// SubjectQueryAggregate is the single request-reply subject for server-side
// aggregations. The entity is specified in the Query body. This subject is
// intentionally separate from the per-entity query subjects so old clients
// that only handle data/done/error are unaffected.
const SubjectQueryAggregate = "query.aggregate"

// Daka-Query-Status header values used in query reply messages.
const (
	HeaderQueryStatus = "Daka-Query-Status"
	StatusData        = "data"
	StatusDone        = "done"
	StatusError       = "error"
	// StatusAggregation is sent as a single reply carrying the aggregation
	// result tree before the final StatusDone sentinel.
	StatusAggregation = "aggregation"
)
