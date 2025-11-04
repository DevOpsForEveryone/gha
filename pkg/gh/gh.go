package gh

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func GetToken(ctx context.Context, workingDirectory string) (string, error) {
	// First try environment variable
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return token, nil
	}

	// Try to get token from git credential helper
	token, err := getTokenFromGitCredentials(ctx, workingDirectory)
	if err == nil && token != "" {
		return token, nil
	}

	return "", fmt.Errorf("no GitHub token found. Please set GITHUB_TOKEN environment variable or configure git credentials for github.com")
}

func getTokenFromGitCredentials(ctx context.Context, workingDirectory string) (string, error) {
	// Locate the 'git' executable
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git executable not found: %w", err)
	}

	// Use git credential fill to get stored credentials for github.com
	cmd := exec.CommandContext(ctx, gitPath, "credential", "fill")
	cmd.Dir = workingDirectory

	// Provide the credential request
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git credential fill failed: %w", err)
	}

	// Parse the credential response
	scanner := bufio.NewScanner(&out)
	var password string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "password=") {
			password = strings.TrimPrefix(line, "password=")
		}
	}

	// For GitHub, the password field contains the token/PAT
	if password != "" {
		return password, nil
	}

	return "", fmt.Errorf("no password/token found in git credentials for github.com")
}
