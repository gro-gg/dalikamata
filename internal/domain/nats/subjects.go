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
)

// Daka-Query-Status header values used in query reply messages.
const (
	HeaderQueryStatus = "Daka-Query-Status"
	StatusData        = "data"
	StatusDone        = "done"
	StatusError       = "error"
)
