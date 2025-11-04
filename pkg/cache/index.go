package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/DevOpsForEveryone/gha/pkg/gh"
)

type IndexCache struct {
	Workflows map[string]int64            `json:"workflows"` // index -> workflow_id
	Runs      map[string]int64            `json:"runs"`      // index -> run_id
	RunJobs   map[string]map[string]int64 `json:"run_jobs"`  // run_id -> (index -> job_id)
	Timestamp time.Time                   `json:"timestamp"`
	mu        sync.RWMutex                `json:"-"`
}

var globalCache *IndexCache
var cacheMutex sync.Mutex

func GetCache() *IndexCache {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	if globalCache == nil {
		globalCache = &IndexCache{
			Workflows: make(map[string]int64),
			Runs:      make(map[string]int64),
			RunJobs:   make(map[string]map[string]int64),
			Timestamp: time.Now(),
		}
		loadCache()
	}
	return globalCache
}

func (c *IndexCache) StoreWorkflows(workflows []gh.Workflow) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Workflows = make(map[string]int64)
	for i, workflow := range workflows {
		c.Workflows[strconv.Itoa(i+1)] = workflow.ID
	}
	c.Timestamp = time.Now()
	saveCache()
}

func (c *IndexCache) StoreRuns(runs []gh.WorkflowRun) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Runs = make(map[string]int64)
	for i, run := range runs {
		c.Runs[strconv.Itoa(i+1)] = run.ID
	}
	c.Timestamp = time.Now()
	saveCache()
}

func (c *IndexCache) StoreJobs(runID int64, jobs []gh.Job) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.RunJobs == nil {
		c.RunJobs = make(map[string]map[string]int64)
	}

	runIDStr := strconv.FormatInt(runID, 10)
	c.RunJobs[runIDStr] = make(map[string]int64)
	for i, job := range jobs {
		c.RunJobs[runIDStr][strconv.Itoa(i+1)] = job.ID
	}
	c.Timestamp = time.Now()
	saveCache()
}

func (c *IndexCache) ResolveWorkflowID(input string) (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try to parse as index first (for small numbers)
	if _, err := strconv.Atoi(input); err == nil {
		if workflowID, exists := c.Workflows[input]; exists {
			return workflowID, nil
		}
	}

	// Try to parse as direct ID
	if id, err := strconv.ParseInt(input, 10, 64); err == nil {
		return id, nil
	}

	return 0, fmt.Errorf("workflow index %s not found (use 'gha actions list' to see available workflows)", input)
}

func (c *IndexCache) ResolveRunID(input string) (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try to parse as index first (for small numbers)
	if _, err := strconv.Atoi(input); err == nil {
		if runID, exists := c.Runs[input]; exists {
			return runID, nil
		}
	}

	// Try to parse as direct ID
	if id, err := strconv.ParseInt(input, 10, 64); err == nil {
		return id, nil
	}

	return 0, fmt.Errorf("run index %s not found (use 'gha actions runs' to see available runs)", input)
}

func (c *IndexCache) ResolveJobID(runID int64, input string) (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try to parse as index first (for small numbers)
	if _, err := strconv.Atoi(input); err == nil {
		runIDStr := strconv.FormatInt(runID, 10)
		if runJobs, exists := c.RunJobs[runIDStr]; exists {
			if jobID, exists := runJobs[input]; exists {
				return jobID, nil
			}
		}
	}

	// Try to parse as direct ID
	if id, err := strconv.ParseInt(input, 10, 64); err == nil {
		return id, nil
	}

	return 0, fmt.Errorf("job index %s not found for run %d (use 'gha actions jobs %d' to see available jobs)", input, runID, runID)
}

func getCacheDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".cache", "gha")
}

func getCacheFile() string {
	return filepath.Join(getCacheDir(), "index_cache.json")
}

func loadCache() {
	cacheFile := getCacheFile()
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return // Cache file doesn't exist or can't be read
	}

	if err := json.Unmarshal(data, globalCache); err != nil {
		// If unmarshaling fails, reset to empty cache
		globalCache.Workflows = make(map[string]int64)
		globalCache.Runs = make(map[string]int64)
		globalCache.RunJobs = make(map[string]map[string]int64)
		globalCache.Timestamp = time.Now()
	}
}

func saveCache() {
	cacheDir := getCacheDir()
	os.MkdirAll(cacheDir, 0755)

	cacheFile := getCacheFile()
	data, err := json.MarshalIndent(globalCache, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(cacheFile, data, 0644)
}
