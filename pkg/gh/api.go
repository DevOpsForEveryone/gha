package gh

import (
	"bytes"
	"context"
	"encoding/base64"
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
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"started_at"`
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

func (c *Client) makeRequestWithBody(ctx context.Context, method, url string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

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

// RerunWorkflow reruns a workflow run
func (c *Client) RerunWorkflow(ctx context.Context, owner, repo string, runID int64) error {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/rerun", c.baseURL, owner, repo, runID)

	resp, err := c.makeRequestWithBody(ctx, "POST", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	return nil
}

// RerunFailedJobs reruns only the failed jobs in a workflow run
func (c *Client) RerunFailedJobs(ctx context.Context, owner, repo string, runID int64) error {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/rerun-failed-jobs", c.baseURL, owner, repo, runID)

	resp, err := c.makeRequestWithBody(ctx, "POST", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	return nil
}

// GetWorkflowContent gets the workflow file content to check for workflow_dispatch
func (c *Client) GetWorkflowContent(ctx context.Context, owner, repo, path string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.baseURL, owner, repo, path)

	resp, err := c.makeRequest(ctx, "GET", url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var content struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if err := json.Unmarshal(body, &content); err != nil {
		return "", err
	}

	if content.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(content.Content)
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}

	return content.Content, nil
}

// TriggerWorkflow triggers a workflow dispatch event
func (c *Client) TriggerWorkflow(ctx context.Context, owner, repo string, workflowID interface{}, ref string, inputs map[string]string) error {
	var url string
	switch v := workflowID.(type) {
	case int64:
		url = fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%d/dispatches", c.baseURL, owner, repo, v)
	case string:
		url = fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/dispatches", c.baseURL, owner, repo, v)
	default:
		return fmt.Errorf("workflowID must be int64 or string")
	}

	payload := map[string]interface{}{
		"ref": ref,
	}
	if len(inputs) > 0 {
		payload["inputs"] = inputs
	}

	resp, err := c.makeRequestWithBody(ctx, "POST", url, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	return nil
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
