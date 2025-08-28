package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

type WorkflowRun struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	HeadBranch string    `json:"head_branch"`
	HeadSHA    string    `json:"head_sha"`
	Event      string    `json:"event"`
	WorkflowID int64     `json:"workflow_id"`
}

type Workflow struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
}

type Job struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	StartedAt  time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

type WorkflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

type WorkflowsResponse struct {
	TotalCount int        `json:"total_count"`
	Workflows  []Workflow `json:"workflows"`
}

type JobsResponse struct {
	TotalCount int   `json:"total_count"`
	Jobs       []Job `json:"jobs"`
}

func NewClient(token string) *Client {
	return &Client{
		token:   token,
		baseURL: "https://api.github.com",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) makeRequest(ctx context.Context, method, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	return c.httpClient.Do(req)
}

func (c *Client) GetWorkflows(ctx context.Context, owner, repo string) ([]Workflow, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows", c.baseURL, owner, repo)
	
	resp, err := c.makeRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response WorkflowsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.Workflows, nil
}

func (c *Client) GetWorkflowRuns(ctx context.Context, owner, repo string, workflowID int64) ([]WorkflowRun, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%d/runs", c.baseURL, owner, repo, workflowID)
	
	resp, err := c.makeRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response WorkflowRunsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.WorkflowRuns, nil
}

func (c *Client) GetAllWorkflowRuns(ctx context.Context, owner, repo string) ([]WorkflowRun, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs", c.baseURL, owner, repo)
	
	resp, err := c.makeRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response WorkflowRunsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.WorkflowRuns, nil
}

func (c *Client) GetJobs(ctx context.Context, owner, repo string, runID int64) ([]Job, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/jobs", c.baseURL, owner, repo, runID)
	
	resp, err := c.makeRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response JobsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.Jobs, nil
}

func (c *Client) GetLogs(ctx context.Context, owner, repo string, runID int64) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/logs", c.baseURL, owner, repo, runID)
	
	resp, err := c.makeRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func ParseRepoFromRemote(remoteURL string) (owner, repo string, err error) {
	// Handle both HTTPS and SSH URLs
	if strings.HasPrefix(remoteURL, "git@github.com:") {
		// SSH format: git@github.com:owner/repo.git
		parts := strings.TrimPrefix(remoteURL, "git@github.com:")
		parts = strings.TrimSuffix(parts, ".git")
		repoParts := strings.Split(parts, "/")
		if len(repoParts) != 2 {
			return "", "", fmt.Errorf("invalid SSH URL format")
		}
		return repoParts[0], repoParts[1], nil
	} else if strings.HasPrefix(remoteURL, "https://github.com/") {
		// HTTPS format: https://github.com/owner/repo.git
		parts := strings.TrimPrefix(remoteURL, "https://github.com/")
		parts = strings.TrimSuffix(parts, ".git")
		repoParts := strings.Split(parts, "/")
		if len(repoParts) != 2 {
			return "", "", fmt.Errorf("invalid HTTPS URL format")
		}
		return repoParts[0], repoParts[1], nil
	}
	
	return "", "", fmt.Errorf("unsupported URL format")
}