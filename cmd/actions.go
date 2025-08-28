package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/DevOpsForEveryone/gha/pkg/gh"
)

func createActionsCommand() *cobra.Command {
	actionsCmd := &cobra.Command{
		Use:   "actions",
		Short: "View GitHub Actions workflows, runs, and logs",
		Long:  "View GitHub Actions workflows, runs, statuses, and logs from GitHub API",
	}

	// Add platform flag to prevent conflicts with .gharc config
	actionsCmd.PersistentFlags().StringArrayP("platform", "P", []string{}, "custom image to use per platform (ignored for actions commands)")

	// List workflows
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all workflows in the repository",
		RunE:  runListWorkflows,
	}

	// List runs
	runsCmd := &cobra.Command{
		Use:   "runs [workflow-id]",
		Short: "List workflow runs (optionally for a specific workflow)",
		RunE:  runListRuns,
	}
	runsCmd.Flags().IntP("limit", "l", 10, "Limit number of runs to display")
	runsCmd.Flags().StringP("status", "s", "", "Filter by status (queued, in_progress, completed)")
	runsCmd.Flags().StringP("branch", "b", "", "Filter by branch name")

	// Show jobs
	jobsCmd := &cobra.Command{
		Use:   "jobs <run-id>",
		Short: "Show jobs for a specific workflow run",
		Args:  cobra.ExactArgs(1),
		RunE:  runShowJobs,
	}

	// Show logs
	logsCmd := &cobra.Command{
		Use:   "logs <run-id>",
		Short: "Show logs for a specific workflow run",
		Args:  cobra.ExactArgs(1),
		RunE:  runShowLogs,
	}
	logsCmd.Flags().StringP("job", "j", "", "Show logs for specific job ID only")
	logsCmd.Flags().BoolP("raw", "r", false, "Show raw logs without formatting")

	// Show detailed run info
	showCmd := &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show detailed information about a workflow run",
		Args:  cobra.ExactArgs(1),
		RunE:  runShowRun,
	}

	// Watch runs in real-time
	watchCmd := &cobra.Command{
		Use:   "watch [workflow-id]",
		Short: "Watch workflow runs in real-time",
		RunE:  runWatchRuns,
	}
	watchCmd.Flags().IntP("interval", "i", 5, "Refresh interval in seconds")

	actionsCmd.AddCommand(listCmd, runsCmd, jobsCmd, logsCmd, showCmd, watchCmd)
	return actionsCmd
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

	fmt.Printf("\n🔄 Workflows for %s/%s\n\n", owner, repo)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tNAME\tSTATE\tPATH\tLAST RUN\n")

	for _, workflow := range workflows {
		// Get latest run for this workflow
		runs, _ := client.GetWorkflowRuns(ctx, owner, repo, workflow.ID)
		lastRun := "Never"
		if len(runs) > 0 {
			lastRun = formatTimeAgo(runs[0].CreatedAt)
		}

		stateIcon := getStateIcon(workflow.State)
		fmt.Fprintf(w, "%d\t%s\t%s %s\t%s\t%s\n",
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
		// Get runs for specific workflow
		var workflowID int64
		if _, err := fmt.Sscanf(args[0], "%d", &workflowID); err != nil {
			return fmt.Errorf("invalid workflow ID: %s", args[0])
		}
		runs, err = client.GetWorkflowRuns(ctx, owner, repo, workflowID)
		fmt.Printf("\n🏃 Workflow runs for workflow %d in %s/%s\n\n", workflowID, owner, repo)
	} else {
		// Get all runs
		runs, err = client.GetAllWorkflowRuns(ctx, owner, repo)
		fmt.Printf("\n🏃 All workflow runs for %s/%s\n\n", owner, repo)
	}

	if err != nil {
		return fmt.Errorf("failed to get workflow runs: %w", err)
	}

	// Apply filters
	filteredRuns := filterRuns(runs, statusFilter, branchFilter, limit)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "RUN ID\tWORKFLOW\tSTATUS\tCONCLUSION\tBRANCH\tEVENT\tCREATED\tDURATION\n")

	for _, run := range filteredRuns {
		conclusion := run.Conclusion
		if conclusion == "" {
			conclusion = "-"
		}

		statusIcon := getStatusIcon(run.Status, conclusion)
		duration := calculateDuration(run.CreatedAt, run.UpdatedAt)

		fmt.Fprintf(w, "%d\t%s\t%s %s\t%s %s\t%s\t%s\t%s\t%s\n",
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

	var runID int64
	if _, err := fmt.Sscanf(args[0], "%d", &runID); err != nil {
		return fmt.Errorf("invalid run ID: %s", args[0])
	}

	client := gh.NewClient(token)
	jobs, err := client.GetJobs(ctx, owner, repo, runID)
	if err != nil {
		return fmt.Errorf("failed to get jobs: %w", err)
	}

	fmt.Printf("\n💼 Jobs for run %d\n\n", runID)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "JOB ID\tNAME\tSTATUS\tCONCLUSION\tSTARTED\tCOMPLETED\tDURATION\n")

	for _, job := range jobs {
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

		fmt.Fprintf(w, "%d\t%s\t%s %s\t%s %s\t%s\t%s\t%s\n",
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

	var runID int64
	if _, err := fmt.Sscanf(args[0], "%d", &runID); err != nil {
		return fmt.Errorf("invalid run ID: %s", args[0])
	}

	client := gh.NewClient(token)
	logsData, err := client.GetLogs(ctx, owner, repo, runID)
	if err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}

	fmt.Printf("Logs for run %d:\n\n", runID)

	// Extract and display logs from ZIP file
	if err := extractAndDisplayLogs(logsData); err != nil {
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

	var runID int64
	if _, err := fmt.Sscanf(args[0], "%d", &runID); err != nil {
		return fmt.Errorf("invalid run ID: %s", args[0])
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
			// Watch specific workflow
			var workflowID int64
			if _, err := fmt.Sscanf(args[0], "%d", &workflowID); err != nil {
				return fmt.Errorf("invalid workflow ID: %s", args[0])
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
