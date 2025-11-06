package cmd

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/DevOpsForEveryone/gha/pkg/cache"
	"github.com/DevOpsForEveryone/gha/pkg/gh"
)

type WorkflowInput struct {
	Description string
	Required    bool
	Default     string
	Type        string
	Options     []string
}

func createActionsCommand() *cobra.Command {
	actionsCmd := &cobra.Command{
		Use:   "actions",
		Short: "View GitHub Actions workflows, runs, and logs",
		Long:  "View GitHub Actions workflows, runs, statuses, and logs from GitHub API",
		RunE:  runActionsWithFlags,
	}

	// Hide global flags from help output
	hideGlobalFlags(actionsCmd)

	// Add platform flag to prevent conflicts with .gharc config
	actionsCmd.PersistentFlags().StringArrayP("platform", "P", []string{}, "custom image to use per platform (ignored for actions commands)")

	// Add flag-style commands
	actionsCmd.Flags().Bool("list", false, "List all workflows in the repository")
	actionsCmd.Flags().Bool("runs", false, "List workflow runs")
	actionsCmd.Flags().String("runs-for", "", "List workflow runs for a specific workflow by ID or index")
	actionsCmd.Flags().String("jobs", "", "Show jobs for a specific workflow run (by ID or index)")
	actionsCmd.Flags().String("logs", "", "Show logs for a specific workflow run (by ID or index)")
	actionsCmd.Flags().String("show", "", "Show detailed information about a workflow run (by ID or index)")
	actionsCmd.Flags().Bool("watch", false, "Watch workflow runs in real-time")
	actionsCmd.Flags().String("watch-for", "", "Watch workflow runs for a specific workflow by ID or index")

	// Additional flags for runs command
	actionsCmd.Flags().IntP("limit", "l", 10, "Limit number of runs to display")
	actionsCmd.Flags().StringP("status", "s", "", "Filter by status (queued, in_progress, completed)")
	actionsCmd.Flags().StringP("branch", "b", "", "Filter by branch name")

	// Additional flags for logs command
	actionsCmd.Flags().StringP("job", "j", "", "Show logs for specific job ID or index only")
	actionsCmd.Flags().String("step", "", "Show logs for specific step name only")
	actionsCmd.Flags().BoolP("raw", "r", false, "Show raw logs without formatting")
	actionsCmd.Flags().BoolP("timestamps", "t", true, "Show timestamps (default: true)")

	// Additional flags for watch command
	actionsCmd.Flags().IntP("interval", "i", 5, "Refresh interval in seconds")

	// New flags for rerun and trigger functionality
	actionsCmd.Flags().String("rerun", "", "Rerun a workflow run (by ID or index)")
	actionsCmd.Flags().Bool("rerun-failed", false, "Rerun only failed jobs in the specified run")
	actionsCmd.Flags().String("trigger", "", "Trigger a workflow by name or ID")
	actionsCmd.Flags().StringP("ref", "", "main", "Git reference (branch/tag) to trigger workflow on")

	// List workflows
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all workflows in the repository",
		RunE:  runListWorkflows,
	}

	// List runs
	runsCmd := &cobra.Command{
		Use:   "runs [workflow-id|index]",
		Short: "List workflow runs (optionally for a specific workflow by ID or index)",
		RunE:  runListRuns,
	}
	runsCmd.Flags().IntP("limit", "l", 10, "Limit number of runs to display")
	runsCmd.Flags().StringP("status", "s", "", "Filter by status (queued, in_progress, completed)")
	runsCmd.Flags().StringP("branch", "b", "", "Filter by branch name")

	// Show jobs
	jobsCmd := &cobra.Command{
		Use:   "jobs <run-id|index>",
		Short: "Show jobs for a specific workflow run (by ID or index)",
		Args:  cobra.ExactArgs(1),
		RunE:  runShowJobs,
	}

	// Show logs
	logsCmd := &cobra.Command{
		Use:   "logs <run-id|index>",
		Short: "Show logs for a specific workflow run (by ID or index)",
		Args:  cobra.ExactArgs(1),
		RunE:  runShowLogs,
	}
	logsCmd.Flags().StringP("job", "j", "", "Show logs for specific job ID or index only")
	logsCmd.Flags().String("step", "", "Show logs for specific step name only")
	logsCmd.Flags().BoolP("raw", "r", false, "Show raw logs without formatting")
	logsCmd.Flags().BoolP("timestamps", "t", true, "Show timestamps (default: true)")

	// Show detailed run info
	showCmd := &cobra.Command{
		Use:   "show <run-id|index>",
		Short: "Show detailed information about a workflow run (by ID or index)",
		Args:  cobra.ExactArgs(1),
		RunE:  runShowRun,
	}

	// Watch runs in real-time
	watchCmd := &cobra.Command{
		Use:   "watch [workflow-id|index]",
		Short: "Watch workflow runs in real-time (optionally for a specific workflow by ID or index)",
		RunE:  runWatchRuns,
	}
	watchCmd.Flags().IntP("interval", "i", 5, "Refresh interval in seconds")

	// Rerun workflow
	rerunCmd := &cobra.Command{
		Use:   "rerun <run-id|index>",
		Short: "Rerun a workflow run (by ID or index)",
		Args:  cobra.ExactArgs(1),
		RunE:  runRerunWorkflow,
	}
	rerunCmd.Flags().Bool("failed-only", false, "Rerun only failed jobs")

	// Trigger workflow
	triggerCmd := &cobra.Command{
		Use:   "trigger <workflow-name|id|index>",
		Short: "Trigger a workflow manually",
		Args:  cobra.ExactArgs(1),
		RunE:  runTriggerWorkflow,
	}
	triggerCmd.Flags().StringP("ref", "", "main", "Git reference (branch/tag) to trigger workflow on")

	actionsCmd.AddCommand(listCmd, runsCmd, jobsCmd, logsCmd, showCmd, watchCmd, rerunCmd, triggerCmd)
	return actionsCmd
}

func runActionsWithFlags(cmd *cobra.Command, args []string) error {
	// Check which flag was used and route to appropriate function
	if listFlag, _ := cmd.Flags().GetBool("list"); listFlag {
		return runListWorkflows(cmd, args)
	}

	if runsFlag, _ := cmd.Flags().GetString("runs"); runsFlag != "" || cmd.Flags().Changed("runs") {
		// If runs flag has a value, add it to args
		if runsFlag != "" {
			args = append(args, runsFlag)
		}
		return runListRuns(cmd, args)
	}

	if jobsFlag, _ := cmd.Flags().GetString("jobs"); jobsFlag != "" {
		return runShowJobs(cmd, []string{jobsFlag})
	}

	if logsFlag, _ := cmd.Flags().GetString("logs"); logsFlag != "" {
		return runShowLogs(cmd, []string{logsFlag})
	}

	if showFlag, _ := cmd.Flags().GetString("show"); showFlag != "" {
		return runShowRun(cmd, []string{showFlag})
	}

	if watchFlag, _ := cmd.Flags().GetString("watch"); watchFlag != "" || cmd.Flags().Changed("watch") {
		// If watch flag has a value, add it to args
		if watchFlag != "" {
			args = append(args, watchFlag)
		}
		return runWatchRuns(cmd, args)
	}

	if rerunFlag, _ := cmd.Flags().GetString("rerun"); rerunFlag != "" {
		return runRerunWorkflow(cmd, []string{rerunFlag})
	}

	if triggerFlag, _ := cmd.Flags().GetString("trigger"); triggerFlag != "" {
		return runTriggerWorkflow(cmd, []string{triggerFlag})
	}

	// If no flags were used, show help
	return cmd.Help()
}

func getRepoInfo() (owner, repo string, err error) {
	// Get remote URL using git command
	cmd := exec.Command("git", "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get git remote URL: %w", err)
	}

	remoteURL := strings.TrimSpace(string(output))
	return gh.ParseRepoFromRemote(remoteURL)
}

func runListWorkflows(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	token, err := gh.GetToken(ctx, ".")
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	owner, repo, err := getRepoInfo()
	if err != nil {
		return err
	}

	client := gh.NewClient(token)
	workflows, err := client.GetWorkflows(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get workflows: %w", err)
	}

	// Store workflows in cache for index lookup
	cache := cache.GetCache()
	cache.StoreWorkflows(workflows)

	fmt.Printf("\n🔄 Workflows for %s/%s\n\n", owner, repo)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "#\tID\tNAME\tSTATE\tPATH\tLAST RUN\n")

	for i, workflow := range workflows {
		// Get latest run for this workflow
		runs, _ := client.GetWorkflowRuns(ctx, owner, repo, workflow.ID)
		lastRun := "Never"
		if len(runs) > 0 {
			lastRun = formatTimeAgo(runs[0].CreatedAt)
		}

		stateIcon := getStateIcon(workflow.State)
		fmt.Fprintf(w, "%d\t%d\t%s\t%s %s\t%s\t%s\n",
			i+1,
			workflow.ID,
			workflow.Name,
			stateIcon,
			workflow.State,
			workflow.Path,
			lastRun)
	}

	w.Flush()
	fmt.Printf("\n📊 Total: %d workflows\n", len(workflows))

	// Show trigger options for active workflows
	showWorkflowTriggerOptions(workflows)
	return nil
}

func runListRuns(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	token, err := gh.GetToken(ctx, ".")
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	owner, repo, err := getRepoInfo()
	if err != nil {
		return err
	}

	// Get flags
	limit, _ := cmd.Flags().GetInt("limit")
	statusFilter, _ := cmd.Flags().GetString("status")
	branchFilter, _ := cmd.Flags().GetString("branch")

	client := gh.NewClient(token)
	var runs []gh.WorkflowRun

	if len(args) > 0 {
		// Get runs for specific workflow - support both ID and index
		cache := cache.GetCache()
		workflowID, err := cache.ResolveWorkflowID(args[0])
		if err != nil {
			return err
		}
		runs, err = client.GetWorkflowRuns(ctx, owner, repo, workflowID)
		if err != nil {
			return fmt.Errorf("failed to get workflow runs: %w", err)
		}
		fmt.Printf("\n🏃 Workflow runs for workflow %d in %s/%s\n\n", workflowID, owner, repo)
	} else {
		// Get all runs
		runs, err = client.GetAllWorkflowRuns(ctx, owner, repo)
		if err != nil {
			return fmt.Errorf("failed to get workflow runs: %w", err)
		}
		fmt.Printf("\n🏃 All workflow runs for %s/%s\n\n", owner, repo)
	}

	// Store all runs in cache for index lookup (before filtering)
	cache := cache.GetCache()
	cache.StoreRuns(runs)

	// Apply filters for display
	filteredRuns := filterRuns(runs, statusFilter, branchFilter, limit)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "#\tRUN ID\tWORKFLOW\tSTATUS\tCONCLUSION\tBRANCH\tEVENT\tCREATED\tDURATION\n")

	for i, run := range filteredRuns {
		conclusion := run.Conclusion
		if conclusion == "" {
			conclusion = "-"
		}

		statusIcon := getStatusIcon(run.Status, conclusion)
		duration := calculateDuration(run.CreatedAt, run.UpdatedAt)

		fmt.Fprintf(w, "%d\t%d\t%s\t%s %s\t%s %s\t%s\t%s\t%s\t%s\n",
			i+1,
			run.ID,
			run.Name,
			statusIcon,
			run.Status,
			getConclusionIcon(conclusion),
			conclusion,
			run.HeadBranch,
			run.Event,
			formatTimeAgo(run.CreatedAt),
			duration)
	}

	w.Flush()
	fmt.Printf("\n📊 Showing %d of %d runs", len(filteredRuns), len(runs))
	if statusFilter != "" {
		fmt.Printf(" (filtered by status: %s)", statusFilter)
	}
	if branchFilter != "" {
		fmt.Printf(" (filtered by branch: %s)", branchFilter)
	}
	fmt.Println()

	// Show rerun options for failed runs
	showRerunOptions(filteredRuns)
	return nil
}

func runShowJobs(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	token, err := gh.GetToken(ctx, ".")
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	owner, repo, err := getRepoInfo()
	if err != nil {
		return err
	}

	// Resolve run ID from input (supports both ID and index)
	cache := cache.GetCache()
	runID, err := cache.ResolveRunID(args[0])
	if err != nil {
		return err
	}

	client := gh.NewClient(token)
	jobs, err := client.GetJobs(ctx, owner, repo, runID)
	if err != nil {
		return fmt.Errorf("failed to get jobs: %w", err)
	}

	// Store jobs in cache for index lookup
	cache.StoreJobs(runID, jobs)

	fmt.Printf("\n💼 Jobs for run %d\n\n", runID)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "#\tJOB ID\tNAME\tSTATUS\tCONCLUSION\tSTARTED\tCOMPLETED\tDURATION\n")

	for i, job := range jobs {
		conclusion := job.Conclusion
		if conclusion == "" {
			conclusion = "-"
		}

		started := "-"
		if !job.StartedAt.IsZero() {
			started = formatTimeAgo(job.StartedAt)
		}

		completed := "-"
		if !job.CompletedAt.IsZero() {
			completed = formatTimeAgo(job.CompletedAt)
		}

		duration := "-"
		if !job.StartedAt.IsZero() && !job.CompletedAt.IsZero() {
			duration = calculateDuration(job.StartedAt, job.CompletedAt)
		}

		statusIcon := getStatusIcon(job.Status, conclusion)

		fmt.Fprintf(w, "%d\t%d\t%s\t%s %s\t%s %s\t%s\t%s\t%s\n",
			i+1,
			job.ID,
			job.Name,
			statusIcon,
			job.Status,
			getConclusionIcon(conclusion),
			conclusion,
			started,
			completed,
			duration)
	}

	w.Flush()
	fmt.Printf("\n📊 Total: %d jobs\n", len(jobs))

	// Show rerun options for failed jobs
	showJobRerunOptions(runID, jobs)
	return nil
}

func runShowLogs(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	token, err := gh.GetToken(ctx, ".")
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	owner, repo, err := getRepoInfo()
	if err != nil {
		return err
	}

	// Resolve run ID from input (supports both ID and index)
	cache := cache.GetCache()
	runID, err := cache.ResolveRunID(args[0])
	if err != nil {
		return err
	}

	// Get flag values
	jobFilter, _ := cmd.Flags().GetString("job")
	stepFilter, _ := cmd.Flags().GetString("step")
	rawOutput, _ := cmd.Flags().GetBool("raw")
	showTimestamps, _ := cmd.Flags().GetBool("timestamps")

	client := gh.NewClient(token)

	// If job filtering is requested, resolve job ID from index if needed
	var resolvedJobID int64
	if jobFilter != "" {
		resolvedJobID, err = cache.ResolveJobID(runID, jobFilter)
		if err != nil {
			return fmt.Errorf("failed to resolve job: %w", err)
		}
	}

	// Get jobs for ordering information
	jobs, err := client.GetJobs(ctx, owner, repo, runID)
	if err != nil {
		return fmt.Errorf("failed to get jobs: %w", err)
	}

	logsData, err := client.GetLogs(ctx, owner, repo, runID)
	if err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}

	// Show header with job info if filtering by job
	var jobInfo string
	if resolvedJobID != 0 {
		for _, job := range jobs {
			if job.ID == resolvedJobID {
				jobInfo = fmt.Sprintf(", Job ID: %d (%s)", job.ID, job.Name)
				break
			}
		}
	}

	// Always show the header (even with --raw) when job filtering is used
	if resolvedJobID != 0 || !rawOutput {
		fmt.Printf("Logs for run %d%s:\n\n", runID, jobInfo)
	}

	// Extract and display logs with improved ordering and filtering
	if err := extractAndDisplayLogsImproved(logsData, jobs, resolvedJobID, stepFilter, rawOutput, showTimestamps); err != nil {
		return fmt.Errorf("failed to extract logs: %w", err)
	}

	return nil
}

func extractAndDisplayLogs(zipData []byte) error {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return err
	}

	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, ".txt") {
			fmt.Printf("=== %s ===\n", file.Name)

			rc, err := file.Open()
			if err != nil {
				fmt.Printf("Error opening %s: %v\n", file.Name, err)
				continue
			}

			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				fmt.Printf("Error reading %s: %v\n", file.Name, err)
				continue
			}

			fmt.Println(string(content))
			fmt.Println()
		}
	}

	return nil
}

type LogFile struct {
	Name    string
	Content string
	JobID   int64
	JobName string
	StepNum int
}

func extractAndDisplayLogsImproved(zipData []byte, jobs []gh.Job, jobFilter int64, stepFilter string, rawOutput bool, showTimestamps bool) error {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return err
	}

	// Create job lookup map
	jobMap := make(map[int64]gh.Job)
	for _, job := range jobs {
		jobMap[job.ID] = job
	}

	// Extract and parse log files
	var logFiles []LogFile
	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, ".txt") {
			rc, err := file.Open()
			if err != nil {
				fmt.Printf("Error opening %s: %v\n", file.Name, err)
				continue
			}

			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				fmt.Printf("Error reading %s: %v\n", file.Name, err)
				continue
			}

			// Parse job ID and step number from filename
			jobID, stepNum := parseLogFileName(file.Name)

			logFile := LogFile{
				Name:    file.Name,
				Content: string(content),
				JobID:   jobID,
				StepNum: stepNum,
			}

			// Add job name if available
			if job, exists := jobMap[jobID]; exists {
				logFile.JobName = job.Name
			}

			logFiles = append(logFiles, logFile)
		}
	}

	// Correlate logs with jobs
	logFiles = correlateLogsWithJobs(logFiles, jobs)

	// Create updated job map after correlation
	updatedJobMap := make(map[int64]gh.Job)
	for _, job := range jobs {
		updatedJobMap[job.ID] = job
	}

	// Sort log files by job start time, then by step number
	sort.Slice(logFiles, func(i, j int) bool {
		logI := logFiles[i]
		logJ := logFiles[j]

		// If same job, sort by step number
		if logI.JobID == logJ.JobID {
			return logI.StepNum < logJ.StepNum
		}

		// Different jobs - sort by job start time
		jobI, existsI := updatedJobMap[logI.JobID]
		jobJ, existsJ := updatedJobMap[logJ.JobID]

		if existsI && existsJ {
			// If start times are different, sort by start time
			if !jobI.StartedAt.Equal(jobJ.StartedAt) {
				return jobI.StartedAt.Before(jobJ.StartedAt)
			}
			// If start times are same, sort by job ID for consistency
			return jobI.ID < jobJ.ID
		}

		// Fallback: sort by step number if job info not available
		return logI.StepNum < logJ.StepNum
	})

	// Display logs with filtering
	for _, logFile := range logFiles {
		// Apply job filter
		if jobFilter != 0 && logFile.JobID != jobFilter {
			continue
		}

		// Apply step filter
		if stepFilter != "" && !strings.Contains(strings.ToLower(logFile.Name), strings.ToLower(stepFilter)) {
			continue
		}

		// Display log file
		if rawOutput {
			fmt.Print(logFile.Content)
		} else {
			jobName := logFile.JobName
			if jobName == "" {
				jobName = fmt.Sprintf("Job %d", logFile.JobID)
			}

			fmt.Printf("=== %s (%s) ===\n", logFile.Name, jobName)

			if showTimestamps {
				fmt.Print(logFile.Content)
			} else {
				// Remove timestamps if requested
				content := logFile.Content
				// Remove BOM if present
				content = strings.TrimPrefix(content, "\xef\xbb\xbf")

				lines := strings.Split(content, "\n")
				timestampRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z\s`)
				for _, line := range lines {
					cleanLine := timestampRegex.ReplaceAllString(line, "")
					fmt.Println(cleanLine)
				}
			}
			fmt.Println()
		}
	}

	return nil
}

func parseLogFileName(filename string) (jobID int64, stepNum int) {
	// Log files are typically named like "0_jobname.txt", "1_stepname.txt", etc.
	// or "jobname/system.txt" for system logs

	// Handle numbered files like "0_jobname.txt", "1_stepname.txt"
	if strings.Contains(filename, "_") {
		parts := strings.Split(strings.TrimSuffix(filename, ".txt"), "_")
		if len(parts) >= 1 {
			if num, err := strconv.Atoi(parts[0]); err == nil {
				stepNum = num
				return 0, stepNum
			}
		}
	}

	// Handle system files like "jobname/system.txt" - assign negative step number to show first
	if strings.Contains(filename, "/system.txt") {
		stepNum = -1 // Negative number to sort system logs before regular steps
	}

	// Return 0 for now - will be corrected in correlateLogsWithJobs
	return 0, stepNum
}

func correlateLogsWithJobs(logFiles []LogFile, jobs []gh.Job) []LogFile {
	// Create a map of job names to job info for correlation
	jobNameMap := make(map[string]gh.Job)
	for _, job := range jobs {
		jobNameMap[strings.ToLower(job.Name)] = job
	}

	for i := range logFiles {
		logFile := &logFiles[i]

		// Try to match by filename patterns
		filename := strings.ToLower(logFile.Name)

		// Check for direct job name matches in filename or path
		for jobName, job := range jobNameMap {
			if strings.Contains(filename, jobName) {
				logFile.JobID = job.ID
				logFile.JobName = job.Name
				break
			}
		}

		// If no match found and it's a numbered file (like "0_something.txt")
		// try to correlate by step number with job order
		if logFile.JobID == 0 && len(jobs) > 1 {
			parts := strings.Split(strings.TrimSuffix(logFile.Name, ".txt"), "_")
			if len(parts) >= 2 {
				// Try to match the second part (job name) with actual job names
				stepName := strings.ToLower(parts[1])
				for jobName, job := range jobNameMap {
					if strings.Contains(jobName, stepName) || strings.Contains(stepName, jobName) {
						logFile.JobID = job.ID
						logFile.JobName = job.Name
						break
					}
				}
			}
		}

		// If still no match, assign to first job as fallback
		if logFile.JobID == 0 && len(jobs) > 0 {
			logFile.JobID = jobs[0].ID
			logFile.JobName = jobs[0].Name
		}
	}

	return logFiles
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func getStatusIcon(status, conclusion string) string {
	switch status {
	case "queued":
		return "🕰️"
	case "in_progress":
		return "🔄"
	case "completed":
		switch conclusion {
		case "success":
			return "✅"
		case "failure":
			return "❌"
		case "cancelled":
			return "🚫"
		case "skipped":
			return "⏭️"
		default:
			return "❓"
		}
	default:
		return "❓"
	}
}

func getConclusionIcon(conclusion string) string {
	switch conclusion {
	case "success":
		return "✅"
	case "failure":
		return "❌"
	case "cancelled":
		return "🚫"
	case "skipped":
		return "⏭️"
	case "neutral":
		return "⚪"
	case "timed_out":
		return "⏰"
	default:
		return ""
	}
}

func getStateIcon(state string) string {
	switch state {
	case "active":
		return "🟢"
	case "disabled":
		return "🔴"
	default:
		return "⚪"
	}
}

func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)
	if duration < time.Minute {
		return "just now"
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm ago", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(duration.Hours()))
	} else {
		return fmt.Sprintf("%dd ago", int(duration.Hours()/24))
	}
}

func calculateDuration(start, end time.Time) string {
	if start.IsZero() || end.IsZero() {
		return "-"
	}
	duration := end.Sub(start)
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm %ds", int(duration.Minutes()), int(duration.Seconds())%60)
	} else {
		return fmt.Sprintf("%dh %dm", int(duration.Hours()), int(duration.Minutes())%60)
	}
}

func filterRuns(runs []gh.WorkflowRun, statusFilter, branchFilter string, limit int) []gh.WorkflowRun {
	var filtered []gh.WorkflowRun

	for _, run := range runs {
		if statusFilter != "" && run.Status != statusFilter {
			continue
		}
		if branchFilter != "" && run.HeadBranch != branchFilter {
			continue
		}
		filtered = append(filtered, run)
		if len(filtered) >= limit {
			break
		}
	}

	return filtered
}

func runShowRun(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	token, err := gh.GetToken(ctx, ".")
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	owner, repo, err := getRepoInfo()
	if err != nil {
		return err
	}

	// Resolve run ID from input (supports both ID and index)
	cache := cache.GetCache()
	runID, err := cache.ResolveRunID(args[0])
	if err != nil {
		return err
	}

	client := gh.NewClient(token)

	// Get all runs to find the specific one
	runs, err := client.GetAllWorkflowRuns(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get workflow runs: %w", err)
	}

	var targetRun *gh.WorkflowRun
	for _, run := range runs {
		if run.ID == runID {
			targetRun = &run
			break
		}
	}

	if targetRun == nil {
		return fmt.Errorf("run %d not found", runID)
	}

	// Get jobs for this run
	jobs, err := client.GetJobs(ctx, owner, repo, runID)
	if err != nil {
		return fmt.Errorf("failed to get jobs: %w", err)
	}

	fmt.Printf("\n📄 Workflow Run Details\n")
	fmt.Printf("\n┌─ Run Information\n")
	fmt.Printf("│ ID: %d\n", targetRun.ID)
	fmt.Printf("│ Workflow: %s\n", targetRun.Name)
	fmt.Printf("│ Status: %s %s\n", getStatusIcon(targetRun.Status, targetRun.Conclusion), targetRun.Status)
	fmt.Printf("│ Conclusion: %s %s\n", getConclusionIcon(targetRun.Conclusion), targetRun.Conclusion)
	fmt.Printf("│ Branch: %s\n", targetRun.HeadBranch)
	fmt.Printf("│ Commit: %s\n", targetRun.HeadSHA[:8])
	fmt.Printf("│ Event: %s\n", targetRun.Event)
	fmt.Printf("│ Created: %s (%s)\n", targetRun.CreatedAt.Format("2006-01-02 15:04:05"), formatTimeAgo(targetRun.CreatedAt))
	fmt.Printf("│ Updated: %s (%s)\n", targetRun.UpdatedAt.Format("2006-01-02 15:04:05"), formatTimeAgo(targetRun.UpdatedAt))
	fmt.Printf("│ Duration: %s\n", calculateDuration(targetRun.CreatedAt, targetRun.UpdatedAt))
	fmt.Printf("└─\n")

	if len(jobs) > 0 {
		fmt.Printf("\n┌─ Jobs (%d)\n", len(jobs))
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "│ JOB ID\tNAME\tSTATUS\tCONCLUSION\tDURATION\n")

		for _, job := range jobs {
			conclusion := job.Conclusion
			if conclusion == "" {
				conclusion = "-"
			}

			duration := calculateDuration(job.StartedAt, job.CompletedAt)
			statusIcon := getStatusIcon(job.Status, conclusion)

			fmt.Fprintf(w, "│ %d\t%s\t%s %s\t%s %s\t%s\n",
				job.ID,
				job.Name,
				statusIcon,
				job.Status,
				getConclusionIcon(conclusion),
				conclusion,
				duration)
		}
		w.Flush()
		fmt.Printf("└─\n")
	}

	fmt.Println()
	return nil
}

func runWatchRuns(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	token, err := gh.GetToken(ctx, ".")
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	owner, repo, err := getRepoInfo()
	if err != nil {
		return err
	}

	interval, _ := cmd.Flags().GetInt("interval")
	client := gh.NewClient(token)

	fmt.Printf("👀 Watching workflow runs for %s/%s (refresh every %ds, press Ctrl+C to stop)\n\n", owner, repo, interval)

	for {
		var runs []gh.WorkflowRun

		if len(args) > 0 {
			// Watch specific workflow - support both ID and index
			cache := cache.GetCache()
			workflowID, err := cache.ResolveWorkflowID(args[0])
			if err != nil {
				return err
			}
			runs, err = client.GetWorkflowRuns(ctx, owner, repo, workflowID)
		} else {
			// Watch all runs
			runs, err = client.GetAllWorkflowRuns(ctx, owner, repo)
		}

		if err != nil {
			fmt.Printf("Error fetching runs: %v\n", err)
		} else {
			// Clear screen and show current time
			fmt.Printf("\033[2J\033[H")
			fmt.Printf("🔄 Last updated: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

			// Show only recent runs (limit to 10)
			limit := 10
			if len(runs) > limit {
				runs = runs[:limit]
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "RUN ID\tWORKFLOW\tSTATUS\tBRANCH\tCREATED\n")

			for _, run := range runs {
				statusIcon := getStatusIcon(run.Status, run.Conclusion)
				fmt.Fprintf(w, "%d\t%s\t%s %s\t%s\t%s\n",
					run.ID,
					run.Name,
					statusIcon,
					run.Status,
					run.HeadBranch,
					formatTimeAgo(run.CreatedAt))
			}
			w.Flush()
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func runRerunWorkflow(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("run ID or index is required")
	}

	ctx := context.Background()
	token, err := gh.GetToken(ctx, ".")
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	owner, repo, err := getRepoInfo()
	if err != nil {
		return err
	}

	// Get the run ID
	runID, err := resolveRunID(ctx, token, owner, repo, args[0])
	if err != nil {
		return err
	}

	// Check if we should rerun only failed jobs
	rerunFailed, _ := cmd.Flags().GetBool("rerun-failed")
	if !rerunFailed {
		// Also check the subcommand flag
		rerunFailed, _ = cmd.Flags().GetBool("failed-only")
	}

	fmt.Printf("🔄 Rerunning workflow run %d", runID)
	if rerunFailed {
		fmt.Print(" (failed jobs only)")
	}
	fmt.Println("...")

	client := gh.NewClient(token)

	// Make the API call to rerun the workflow
	if rerunFailed {
		err = client.RerunFailedJobs(ctx, owner, repo, runID)
	} else {
		err = client.RerunWorkflow(ctx, owner, repo, runID)
	}

	if err != nil {
		return fmt.Errorf("failed to rerun workflow: %w", err)
	}

	fmt.Printf("✅ Successfully triggered rerun for workflow run %d\n", runID)
	fmt.Println("💡 Use 'gha actions runs' to check the status")

	return nil
}

func runTriggerWorkflow(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("workflow name or ID is required")
	}

	ctx := context.Background()
	token, err := gh.GetToken(ctx, ".")
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	owner, repo, err := getRepoInfo()
	if err != nil {
		return err
	}

	// Get workflow ID and details
	workflowID, err := resolveWorkflowID(ctx, token, owner, repo, args[0])
	if err != nil {
		return err
	}

	// Get workflow details to check for inputs
	client := gh.NewClient(token)
	workflows, err := client.GetWorkflows(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get workflows: %w", err)
	}

	var workflowPath string
	var workflowName string
	for _, workflow := range workflows {
		if fmt.Sprintf("%d", workflow.ID) == fmt.Sprintf("%v", workflowID) {
			workflowPath = workflow.Path
			workflowName = workflow.Name
			break
		}
	}

	// Get default ref
	ref, _ := cmd.Flags().GetString("ref")
	inputs := make(map[string]string)

	// Check if workflow has inputs defined
	var workflowInputs map[string]WorkflowInput
	if workflowPath != "" {
		yamlContent, err := client.GetWorkflowContent(ctx, owner, repo, workflowPath)
		if err == nil {
			workflowInputs = parseWorkflowInputs(yamlContent)
		}
	}

	// If workflow has inputs, automatically use interactive mode
	if len(workflowInputs) > 0 {
		fmt.Printf("🎯 Workflow '%s' requires inputs. Starting interactive mode...\n\n", workflowName)
		ref, inputs, err = runInteractiveTrigger(ctx, token, owner, repo, workflowID)
		if err != nil {
			return err
		}
	} else {
		// No inputs needed, get current branch as default if ref not specified
		if ref == "main" {
			cmd := exec.Command("git", "branch", "--show-current")
			output, err := cmd.Output()
			if err == nil {
				currentBranch := strings.TrimSpace(string(output))
				if currentBranch != "" {
					ref = currentBranch
				}
			}
		}

		fmt.Printf("🚀 Triggering workflow '%s' on ref '%s' (no inputs required)\n", workflowName, ref)
	}

	if len(workflowInputs) == 0 {
		fmt.Println("...")
	}

	// Make the API call to trigger the workflow
	err = client.TriggerWorkflow(ctx, owner, repo, workflowID, ref, inputs)
	if err != nil {
		return fmt.Errorf("failed to trigger workflow: %w", err)
	}

	fmt.Printf("✅ Successfully triggered workflow '%s'\n", workflowName)
	fmt.Println("💡 Use 'gha actions runs' to check the status")

	return nil
}

func resolveRunID(ctx context.Context, token, owner, repo, identifier string) (int64, error) {
	// Try to parse as direct ID first
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil {
		return id, nil
	}

	// Try to parse as index (e.g., "1", "2", etc.)
	if index, err := strconv.Atoi(identifier); err == nil && index > 0 {
		// Get recent runs and return the one at the specified index
		client := gh.NewClient(token)
		runs, err := client.GetAllWorkflowRuns(ctx, owner, repo)
		if err != nil {
			return 0, fmt.Errorf("failed to get workflow runs: %w", err)
		}

		if index > len(runs) {
			return 0, fmt.Errorf("run index %d is out of range (max: %d)", index, len(runs))
		}

		return runs[index-1].ID, nil
	}

	return 0, fmt.Errorf("invalid run identifier: %s (use run ID or index)", identifier)
}

func resolveWorkflowID(ctx context.Context, token, owner, repo, identifier string) (interface{}, error) {
	// Try to parse as direct ID first
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil {
		return id, nil
	}

	// Try to parse as index (e.g., "1", "2", etc.)
	if index, err := strconv.Atoi(identifier); err == nil && index > 0 {
		client := gh.NewClient(token)
		workflows, err := client.GetWorkflows(ctx, owner, repo)
		if err != nil {
			return nil, fmt.Errorf("failed to get workflows: %w", err)
		}

		if index > len(workflows) {
			return nil, fmt.Errorf("workflow index %d is out of range (max: %d)", index, len(workflows))
		}

		return workflows[index-1].ID, nil
	}

	// Try as workflow name or filename
	client := gh.NewClient(token)
	workflows, err := client.GetWorkflows(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflows: %w", err)
	}

	// First try exact name match
	for _, workflow := range workflows {
		if workflow.Name == identifier {
			return workflow.ID, nil
		}
	}

	// Then try filename match
	for _, workflow := range workflows {
		if strings.Contains(workflow.Path, identifier) {
			return workflow.ID, nil
		}
	}

	// Finally try partial name match
	for _, workflow := range workflows {
		if strings.Contains(strings.ToLower(workflow.Name), strings.ToLower(identifier)) {
			return workflow.ID, nil
		}
	}

	return nil, fmt.Errorf("workflow not found: %s", identifier)
}

func runInteractiveTrigger(ctx context.Context, token, owner, repo string, workflowID interface{}) (string, map[string]string, error) {
	fmt.Println("🎯 Interactive Workflow Trigger")
	fmt.Println()

	// Get workflow details
	client := gh.NewClient(token)
	workflows, err := client.GetWorkflows(ctx, owner, repo)
	var workflowName string
	var workflowPath string

	if err == nil {
		for _, workflow := range workflows {
			if fmt.Sprintf("%d", workflow.ID) == fmt.Sprintf("%v", workflowID) {
				workflowName = workflow.Name
				workflowPath = workflow.Path
				break
			}
		}
	}

	if workflowName == "" {
		workflowName = fmt.Sprintf("%v", workflowID)
	}

	fmt.Printf("Workflow: %s\n", workflowName)
	if workflowPath != "" {
		fmt.Printf("Path: %s\n", workflowPath)
	}
	fmt.Println()

	// Get current branch as default
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	defaultRef := "main"
	if err == nil {
		defaultRef = strings.TrimSpace(string(output))
	}

	// Ask for ref
	fmt.Printf("Enter git reference (branch/tag) [%s]: ", defaultRef)
	reader := bufio.NewReader(os.Stdin)
	refInput, _ := reader.ReadString('\n')
	ref := strings.TrimSpace(refInput)
	if ref == "" {
		ref = defaultRef
	}

	// Get workflow content and parse inputs
	inputs := make(map[string]string)
	var workflowInputs map[string]WorkflowInput

	if workflowPath != "" {
		yamlContent, err := client.GetWorkflowContent(ctx, owner, repo, workflowPath)
		if err == nil {
			workflowInputs = parseWorkflowInputs(yamlContent)
		}
	}

	if len(workflowInputs) > 0 {
		fmt.Printf("\n📝 Workflow Inputs:\n")
		for inputName, inputDef := range workflowInputs {
			// Show input description
			description := inputDef.Description
			if description == "" {
				description = "No description"
			}

			required := ""
			if inputDef.Required {
				required = " (required)"
			}

			defaultValue := inputDef.Default
			prompt := fmt.Sprintf("%s%s", inputName, required)
			if description != "No description" {
				prompt += fmt.Sprintf(" - %s", description)
			}
			if defaultValue != "" {
				prompt += fmt.Sprintf(" [%s]", defaultValue)
			}

			fmt.Printf("%s: ", prompt)

			// Read input value
			valueInput, _ := reader.ReadString('\n')
			value := strings.TrimSpace(valueInput)

			// Use default if no value provided
			if value == "" && defaultValue != "" {
				value = defaultValue
			}

			// Check required fields
			if inputDef.Required && value == "" {
				fmt.Printf("❌ %s is required. Please provide a value.\n", inputName)
				fmt.Printf("%s: ", prompt)
				valueInput, _ = reader.ReadString('\n')
				value = strings.TrimSpace(valueInput)
			}

			if value != "" {
				inputs[inputName] = value
			}
		}
	} else {
		fmt.Println("\n📝 No workflow inputs defined or unable to parse workflow file.")
		fmt.Println("You can still provide custom inputs if needed.")
		fmt.Println("\nEnter additional inputs (press Enter with empty key to finish):")

		for {
			fmt.Print("Input key: ")
			keyInput, _ := reader.ReadString('\n')
			key := strings.TrimSpace(keyInput)
			if key == "" {
				break
			}

			fmt.Printf("Input value for '%s': ", key)
			valueInput, _ := reader.ReadString('\n')
			value := strings.TrimSpace(valueInput)

			if value != "" {
				inputs[key] = value
			}
		}
	}

	// Confirmation
	fmt.Printf("\n📋 Summary:\n")
	fmt.Printf("  Workflow: %s\n", workflowName)
	fmt.Printf("  Reference: %s\n", ref)
	if len(inputs) > 0 {
		fmt.Printf("  Inputs:\n")
		for k, v := range inputs {
			fmt.Printf("    %s: %s\n", k, v)
		}
	} else {
		fmt.Printf("  Inputs: None\n")
	}

	fmt.Print("\nProceed? (y/N): ")
	confirmInput, _ := reader.ReadString('\n')
	confirm := strings.TrimSpace(confirmInput)
	if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
		return "", nil, fmt.Errorf("cancelled by user")
	}

	return ref, inputs, nil
}

func parseWorkflowInputs(yamlContent string) map[string]WorkflowInput {
	inputs := make(map[string]WorkflowInput)

	// Find the workflow_dispatch section
	lines := strings.Split(yamlContent, "\n")
	inWorkflowDispatch := false
	inInputs := false
	currentInput := ""
	currentInputData := WorkflowInput{}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if we're entering workflow_dispatch section
		if strings.Contains(trimmed, "workflow_dispatch:") {
			inWorkflowDispatch = true
			continue
		}

		// If we're in workflow_dispatch, look for inputs
		if inWorkflowDispatch {
			if strings.Contains(trimmed, "inputs:") {
				inInputs = true
				continue
			}

			// If we hit another top-level key (no indentation), we're done with workflow_dispatch
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" && !strings.HasPrefix(trimmed, "#") && trimmed != "inputs:" {
				break
			}
		}

		// Parse inputs
		if inInputs && inWorkflowDispatch {
			// Determine indentation level
			indentLevel := len(line) - len(strings.TrimLeft(line, " \t"))

			// Input name (should be at inputs level + 2 spaces, ends with colon)
			if indentLevel >= 6 && strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "#") {
				// Check if this looks like an input name (not a property)
				if !strings.Contains(trimmed, "description:") && !strings.Contains(trimmed, "required:") &&
					!strings.Contains(trimmed, "default:") && !strings.Contains(trimmed, "type:") &&
					!strings.Contains(trimmed, "options:") {

					// Save previous input if exists
					if currentInput != "" {
						inputs[currentInput] = currentInputData
					}

					// Start new input
					parts := strings.SplitN(trimmed, ":", 2)
					if len(parts) > 0 {
						currentInput = strings.TrimSpace(parts[0])
						currentInputData = WorkflowInput{}
					}
					continue
				}
			}

			// Parse input properties (should be more indented than input name)
			if currentInput != "" && indentLevel >= 8 {
				if strings.Contains(trimmed, "description:") {
					parts := strings.SplitN(trimmed, ":", 2)
					if len(parts) > 1 {
						currentInputData.Description = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
					}
				} else if strings.Contains(trimmed, "required:") {
					parts := strings.SplitN(trimmed, ":", 2)
					if len(parts) > 1 {
						currentInputData.Required = strings.TrimSpace(parts[1]) == "true"
					}
				} else if strings.Contains(trimmed, "default:") {
					parts := strings.SplitN(trimmed, ":", 2)
					if len(parts) > 1 {
						currentInputData.Default = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
					}
				} else if strings.Contains(trimmed, "type:") {
					parts := strings.SplitN(trimmed, ":", 2)
					if len(parts) > 1 {
						currentInputData.Type = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
					}
				} else if strings.Contains(trimmed, "options:") {
					// Parse options array (simplified)
					for j := i + 1; j < len(lines); j++ {
						optionLine := lines[j]
						optionTrimmed := strings.TrimSpace(optionLine)
						optionIndent := len(optionLine) - len(strings.TrimLeft(optionLine, " \t"))

						if optionIndent <= indentLevel {
							break // End of options
						}

						if strings.HasPrefix(optionTrimmed, "- ") {
							option := strings.TrimSpace(strings.TrimPrefix(optionTrimmed, "- "))
							option = strings.Trim(option, "\"'")
							currentInputData.Options = append(currentInputData.Options, option)
						}
					}
				}
			}
		}
	}

	// Save the last input
	if currentInput != "" {
		inputs[currentInput] = currentInputData
	}

	return inputs
}

func showRerunOptions(runs []gh.WorkflowRun) {
	var failedRuns []gh.WorkflowRun
	var completedRuns []gh.WorkflowRun

	for _, run := range runs {
		if run.Status == "completed" {
			completedRuns = append(completedRuns, run)
			if run.Conclusion == "failure" {
				failedRuns = append(failedRuns, run)
			}
		}
	}

	if len(failedRuns) > 0 {
		fmt.Printf("\n🔄 Quick Rerun Options:\n")
		for i, run := range failedRuns {
			if i >= 3 { // Show only first 3 failed runs
				break
			}
			fmt.Printf("  gha actions --rerun %d              # Rerun all jobs\n", run.ID)
			fmt.Printf("  gha actions --rerun %d --rerun-failed  # Rerun failed jobs only\n", run.ID)
		}
	}

	if len(completedRuns) > 0 && len(failedRuns) == 0 {
		fmt.Printf("\n🔄 Rerun latest completed run:\n")
		fmt.Printf("  gha actions --rerun %d\n", completedRuns[0].ID)
	}

	fmt.Println()
}

func showJobRerunOptions(runID int64, jobs []gh.Job) {
	var failedJobs []gh.Job
	hasFailedJobs := false

	for _, job := range jobs {
		if job.Conclusion == "failure" {
			failedJobs = append(failedJobs, job)
			hasFailedJobs = true
		}
	}

	if hasFailedJobs {
		fmt.Printf("\n🔄 Rerun Options:\n")
		fmt.Printf("  gha actions --rerun %d              # Rerun all jobs\n", runID)
		fmt.Printf("  gha actions --rerun %d --rerun-failed  # Rerun failed jobs only\n", runID)

		if len(failedJobs) <= 3 {
			fmt.Printf("\n❌ Failed jobs:\n")
			for _, job := range failedJobs {
				fmt.Printf("  - %s (ID: %d)\n", job.Name, job.ID)
			}
		}
	} else {
		fmt.Printf("\n🔄 Rerun all jobs:\n")
		fmt.Printf("  gha actions --rerun %d\n", runID)
	}

	fmt.Println()
}

func showWorkflowTriggerOptions(workflows []gh.Workflow) {
	if len(workflows) == 0 {
		return
	}

	ctx := context.Background()
	token, err := gh.GetToken(ctx, ".")
	if err != nil {
		return // Silently skip if we can't get token
	}

	owner, repo, err := getRepoInfo()
	if err != nil {
		return // Silently skip if we can't get repo info
	}

	client := gh.NewClient(token)
	var triggerableWorkflows []gh.Workflow

	// Check each workflow for workflow_dispatch
	for _, workflow := range workflows {
		if workflow.State != "active" {
			continue
		}

		content, err := client.GetWorkflowContent(ctx, owner, repo, workflow.Path)
		if err != nil {
			continue // Skip if we can't get content
		}

		// Check if workflow has workflow_dispatch event
		if strings.Contains(content, "workflow_dispatch") {
			triggerableWorkflows = append(triggerableWorkflows, workflow)
		}
	}

	if len(triggerableWorkflows) == 0 {
		return // No triggerable workflows
	}

	fmt.Printf("\n🚀 Trigger Workflows (with workflow_dispatch):\n")
	for i, workflow := range triggerableWorkflows {
		if i >= 5 { // Show only first 5 workflows
			break
		}
		fmt.Printf("  gha actions --trigger \"%s\"                    # Auto-detects inputs\n", workflow.Name)
		fmt.Printf("  gha actions trigger \"%s\"                      # Same as above\n", workflow.Name)
	}
	fmt.Println()
}
