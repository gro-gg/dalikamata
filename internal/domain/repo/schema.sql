CREATE TABLE IF NOT EXISTS repos (
	repo_id TEXT PRIMARY KEY,
	name    TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS commits (
	sha       TEXT PRIMARY KEY,
	repo_id   TEXT NOT NULL,
	author    TEXT NOT NULL,
	timestamp TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_commits_repo_id ON commits(repo_id);
CREATE TABLE IF NOT EXISTS pull_requests (
	id          TEXT PRIMARY KEY,
	repo_id     TEXT NOT NULL,
	name        TEXT NOT NULL,
	title       TEXT NOT NULL,
	description TEXT NOT NULL,
	state       TEXT NOT NULL,
	author      TEXT NOT NULL,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pull_requests_repo_id ON pull_requests(repo_id);
CREATE TABLE IF NOT EXISTS workflows (
	id       TEXT PRIMARY KEY,
	name     TEXT NOT NULL,
	repo_ids TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS workflow_runs (
	id          TEXT PRIMARY KEY,
	workflow_id TEXT NOT NULL,
	number      INTEGER NOT NULL,
	status      TEXT NOT NULL,
	branch      TEXT NOT NULL,
	commit_sha  TEXT NOT NULL,
	started_at  TEXT NOT NULL,
	duration    REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_workflow_id ON workflow_runs(workflow_id);
CREATE TABLE IF NOT EXISTS workflow_tasks (
	workflow_run_id TEXT NOT NULL,
	task_order      INTEGER NOT NULL,
	name            TEXT NOT NULL,
	status          TEXT NOT NULL,
	started_at      TEXT NOT NULL,
	duration        REAL NOT NULL,
	PRIMARY KEY (workflow_run_id, task_order)
);
CREATE TABLE IF NOT EXISTS teams (
	name TEXT PRIMARY KEY
);
CREATE TABLE IF NOT EXISTS components (
	name      TEXT PRIMARY KEY,
	team_name TEXT NOT NULL,
	repo_ids  TEXT NOT NULL
);
