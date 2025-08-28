package gh

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

func GetToken(ctx context.Context, workingDirectory string) (string, error) {
	var token string

	// Locate the 'gh' executable
	path, err := exec.LookPath("gh")
	if err != nil {
		return "", err
	}

	// Command setup - validate path is safe
	if path == "" {
		return "", fmt.Errorf("gh executable not found")
	}
	cmd := exec.CommandContext(ctx, path, "auth", "token")
	cmd.Dir = workingDirectory

	// Capture the output
	var out bytes.Buffer
	cmd.Stdout = &out

	// Run the command
	err = cmd.Run()
	if err != nil {
		return "", err
	}

	// Read the first line of the output
	scanner := bufio.NewScanner(&out)
	if scanner.Scan() {
		token = scanner.Text()
	}

	return token, nil
}
