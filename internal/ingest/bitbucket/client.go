package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
)

const pageLimit = 100

// BitbucketClient fetches data from a Bitbucket Server instance.
type BitbucketClient interface {
	GetRepos(ctx context.Context, projectKey string) ([]apiRepo, error)
	GetCommits(ctx context.Context, projectKey, repoSlug, sinceSHA string) ([]apiCommit, error)
	GetPullRequests(ctx context.Context, projectKey, repoSlug string) ([]apiPullRequest, error)
	// GetRawFile fetches the raw content of a file at the repo root. found is
	// false (with a nil error) when the file does not exist (HTTP 404).
	GetRawFile(ctx context.Context, projectKey, repoSlug, path string) (content []byte, found bool, err error)
}

// httpClient is the production implementation of BitbucketClient.
type httpClient struct {
	baseURL string
	token   string
	client  *http.Client
	logger  *slog.Logger
}

func NewClient(baseURL, token string, httpCl *http.Client, logger *slog.Logger) BitbucketClient {
	return &httpClient{
		baseURL: baseURL,
		token:   token,
		client:  httpCl,
		logger:  logger.With("connection", "http", "client", "bitbucket"),
	}
}

func (c *httpClient) GetRepos(ctx context.Context, projectKey string) ([]apiRepo, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos", projectKey)
	return paginate[apiRepo](ctx, c, path, nil)
}

func (c *httpClient) GetCommits(ctx context.Context, projectKey, repoSlug, sinceSHA string) ([]apiCommit, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/commits", projectKey, repoSlug)
	var params url.Values
	if sinceSHA != "" {
		params = url.Values{"since": {sinceSHA}}
	}
	return paginate[apiCommit](ctx, c, path, params)
}

func (c *httpClient) GetPullRequests(ctx context.Context, projectKey, repoSlug string) ([]apiPullRequest, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests", projectKey, repoSlug)
	return paginate[apiPullRequest](ctx, c, path, url.Values{"state": []string{"ALL"}})
}

func (c *httpClient) GetRawFile(ctx context.Context, projectKey, repoSlug, path string) ([]byte, bool, error) {
	url := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/raw/%s", c.baseURL, projectKey, repoSlug, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			c.logger.Error("closing response body", "error", cerr)
		}
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("reading raw file %s: %w", path, err)
	}
	return body, true, nil
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
		defer func() {
			err = resp.Body.Close()
			if err != nil {
				c.logger.Error("closing response body", "error", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
		}

		var page pagedResponse[T]
		if err = json.NewDecoder(resp.Body).Decode(&page); err != nil {
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
