package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
)

const pageLimit = 100

// BitbucketClient fetches data from a Bitbucket Server instance.
type BitbucketClient interface {
	GetRepos(ctx context.Context, projectKey string) ([]apiRepo, error)
	GetCommits(ctx context.Context, projectKey, repoSlug string) ([]apiCommit, error)
	GetPullRequests(ctx context.Context, projectKey, repoSlug string) ([]apiPullRequest, error)
}

// httpClient is the production implementation of BitbucketClient.
type httpClient struct {
	baseURL string
	token   string
	client  *http.Client
	logger  *slog.Logger
}

func NewClient(baseURL, token string, logger *slog.Logger) BitbucketClient {
	return &httpClient{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{},
		logger:  logger,
	}
}

func (c *httpClient) GetRepos(ctx context.Context, projectKey string) ([]apiRepo, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos", projectKey)
	return paginate[apiRepo](ctx, c, path, nil)
}

func (c *httpClient) GetCommits(ctx context.Context, projectKey, repoSlug string) ([]apiCommit, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/commits", projectKey, repoSlug)
	return paginate[apiCommit](ctx, c, path, nil)
}

func (c *httpClient) GetPullRequests(ctx context.Context, projectKey, repoSlug string) ([]apiPullRequest, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests", projectKey, repoSlug)
	return paginate[apiPullRequest](ctx, c, path, url.Values{"state": []string{"ALL"}})
}

func paginate[T any](ctx context.Context, c *httpClient, path string, params url.Values) ([]T, error) {
	if params == nil {
		params = url.Values{}
	}

	var all []T
	start := 0

	for {
		params.Set("start", strconv.Itoa(start))
		params.Set("limit", strconv.Itoa(pageLimit))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			if err := resp.Body.Close(); err != nil {
				c.logger.Error("closing response body", "error", err)
			}
			return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
		}

		var page pagedResponse[T]
		err = json.NewDecoder(resp.Body).Decode(&page)
		if err := resp.Body.Close(); err != nil {
			c.logger.Error("closing response body", "error", err)
		}
		if err != nil {
			return nil, err
		}

		all = append(all, page.Values...)

		c.logger.Debug("fetched page", "path", path, "start", start, "count", len(page.Values), "isLastPage", page.IsLastPage)

		if page.IsLastPage {
			break
		}
		start = page.NextPageStart
	}

	return all, nil
}
