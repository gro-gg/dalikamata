package jenkins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

const buildTreeParam = "builds[number,result,timestamp,duration,inProgress,actions[_class,lastBuiltRevision[SHA1,branch[name]],remoteUrls]]"

type JenkinsClient interface {
	GetJobs(ctx context.Context, jobPath string) ([]apiJob, error)
	GetBuilds(ctx context.Context, jobPath string) ([]apiBuild, error)
	GetStages(ctx context.Context, jobPath string, buildNumber int) ([]apiStage, error)
}

type httpClient struct {
	baseURL string
	user    string
	token   string
	client  *http.Client
	logger  *slog.Logger
}

func NewClient(baseURL, user, token string, httpCl *http.Client, logger *slog.Logger) JenkinsClient {
	return &httpClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		user:    user,
		token:   token,
		client:  httpCl,
		logger:  logger.With("connection", "jenkins"),
	}
}

// jobURLPath converts "team/payments/main" → "/job/team/job/payments/job/main".
// Returns "" for an empty path (root).
func jobURLPath(jobPath string) string {
	if jobPath == "" {
		return ""
	}
	parts := strings.Split(jobPath, "/")
	return "/job/" + strings.Join(parts, "/job/")
}

func (c *httpClient) GetJobs(ctx context.Context, jobPath string) ([]apiJob, error) {
	url := c.baseURL + jobURLPath(jobPath) + "/api/json?tree=jobs[_class,name]"

	var list apiJobList
	if err := c.get(ctx, url, &list); err != nil {
		return nil, fmt.Errorf("getting jobs at %q: %w", jobPath, err)
	}
	return list.Jobs, nil
}

func (c *httpClient) GetBuilds(ctx context.Context, jobPath string) ([]apiBuild, error) {
	url := c.baseURL + jobURLPath(jobPath) + "/api/json?tree=" + buildTreeParam

	var list apiBuildList
	if err := c.get(ctx, url, &list); err != nil {
		return nil, fmt.Errorf("getting builds for %q: %w", jobPath, err)
	}
	return list.Builds, nil
}

func (c *httpClient) GetStages(ctx context.Context, jobPath string, buildNumber int) ([]apiStage, error) {
	url := fmt.Sprintf("%s%s/%d/wfapi/describe", c.baseURL, jobURLPath(jobPath), buildNumber)

	var desc apiWFDescribe
	if err := c.get(ctx, url, &desc); err != nil {
		return nil, fmt.Errorf("getting stages for %q build %d: %w", jobPath, buildNumber, err)
	}
	return desc.Stages, nil
}

func (c *httpClient) get(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.SetBasicAuth(c.user, c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Error("closing response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
