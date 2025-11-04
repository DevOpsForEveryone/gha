package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
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

func createActionsCommand() *cobra.Command {
	actionsCmd := &cobra.Command{
		Use:   "actions",
		Short: "View GitHub Actions workflows, runs, and logs",
		Long:  "View GitHub Actions workflows, runs, statuses, and logs from GitHub API",
		RunE:  runActionsWithFlags,
	}

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

	actionsCmd.AddCommand(listCmd, runsCmd, jobsCmd, logsCmd, showCmd, watchCmd)
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
	fmt.Printf("\n📊 Total: %d workflows\n\n", len(workflows))
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
	fmt.Printf("\n📊 Total: %d jobs\n\n", len(jobs))
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
