# GHA - GitHub Actions Runner

[![Version](https://img.shields.io/badge/version-1.0.0-blue.svg)](https://github.com/Leapfrog-DevOps/gha/releases)
[![Go Version](https://img.shields.io/badge/go-1.24+-00ADD8.svg)](https://golang.org)
[![Docker](https://img.shields.io/badge/docker-20.10+-2496ED.svg)](https://docker.com)

Run your [GitHub Actions](https://docs.github.com/en/actions) locally!

GHA enables you to execute GitHub Actions workflows on your local machine using Docker containers. This provides fast feedback during development and allows you to use GitHub Actions as a local task runner, eliminating the need to commit and push changes just to test your workflows.

## Why Use GHA?

- **Fast Feedback** - Test workflow changes locally without committing to GitHub
- **Local Development** - Debug actions and workflows in your development environment
- **Task Runner** - Use GitHub Actions workflows as a replacement for Makefiles
- **Consistent Environment** - Run the same containers and environment variables as GitHub's hosted runners
- **Offline Development** - Work on workflows without internet connectivity

## Table of Contents

- [Quick Start](#quick-start)
- [Installation](#installation)
- [Basic Usage](#basic-usage)
- [Commands Reference](#commands-reference)
  - [Main Command](#main-command)
  - [Actions Commands](#actions-commands)
  - [OIDC Commands](#oidc-commands)
  - [Utility Commands](#utility-commands)
- [Configuration](#configuration)
- [Advanced Features](#advanced-features)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)
- [Development](#development)

## How It Works

GHA reads your GitHub Actions workflows from `.github/workflows/` and determines which actions need to run. It uses the Docker API to pull or build the necessary images as defined in your workflow files, then executes containers for each action. The environment variables and filesystem are configured to match what GitHub provides in their hosted runners.

## Quick Start

1. **Prerequisites**: Ensure Docker is installed and running on your system
2. **Install GHA**: Download the latest binary or build from source (see [Installation](#installation))
3. **Navigate to your repository**: `cd your-repo-with-github-actions`
4. **Run a workflow**: `gha push` (or any event name from your workflows)

```bash
# Example: Run workflows triggered by push event
gha push

# Run a specific job
gha push -j build

# Run with secrets
gha push -s GITHUB_TOKEN=your_token
```

### Example Workflow

Create `.github/workflows/ci.yml`:

```yaml
name: CI
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '18'
      - run: npm install
      - run: npm test

  build:
    runs-on: ubuntu-latest
    needs: test
    steps:
      - uses: actions/checkout@v4
      - run: echo "Building application..."
      - run: echo "Build complete!"
```

Then run it locally:

```bash
# Run all jobs for push event
gha push

# Run only the test job
gha push -j test

# Run with file watching (re-run on changes)
gha push --watch
```

## Installation

### Download Binary

Download the latest release from the [releases page](https://github.com/Leapfrog-DevOps/gha/releases):

```bash
# Linux/macOS
curl -L https://github.com/Leapfrog-DevOps/gha/releases/latest/download/gha-linux-amd64 -o gha
chmod +x gha
sudo mv gha /usr/local/bin/
```

### Build from Source

**Prerequisites:**

- Go 1.24+ ([installation guide](https://golang.org/doc/install))
- Git
- Docker

**Build steps:**

```bash
# Clone the repository
git clone https://github.com/Leapfrog-DevOps/gha.git
cd gha

# Run tests
make test

# Build and install
make install
```

### Verify Installation

```bash
gha --version
```

## Basic Usage

### Running Workflows by Event

GHA automatically detects workflows in `.github/workflows/` and runs them based on event triggers:

```bash
# Run workflows triggered by 'push' event (default)
gha push

# Run workflows triggered by 'pull_request' event
gha pull_request

# If only one event exists, you can omit the event name
gha

# Auto-detect the first event type
gha --detect-event
```

### Running Specific Jobs

Use the `-j` flag to run a specific job by ID:

```bash
# Run only the 'build' job
gha push -j build

# Run only the 'test' job from pull_request workflows
gha pull_request -j test
```

### Using Secrets and Environment Variables

#### Secrets

```bash
# Pass secrets via command line
gha push -s GITHUB_TOKEN=ghp_xxxx -s API_KEY=secret123

# Load secrets from file (.secrets)
echo "GITHUB_TOKEN=ghp_xxxx" > .secrets
echo "API_KEY=secret123" >> .secrets
gha push --secret-file .secrets

# Prompt for secret value (secure input)
gha push -s GITHUB_TOKEN
```

#### Environment Variables (Runtime)

```bash
# Pass environment variables
gha push --env NODE_ENV=production --env DEBUG=true

# Load from .env file
echo "NODE_ENV=production" > .env
echo "DEBUG=true" >> .env
gha push --env-file .env
```

#### Variables (GitHub Variables)

```bash
# Pass variables (similar to GitHub repository variables)
gha push --var ENVIRONMENT=staging --var VERSION=1.0.0

# Load from .vars file
gha push --var-file .vars
```

### File Watching Mode

Automatically re-run workflows when files change:

```bash
# Watch for file changes and re-run
gha push --watch

# Watch with specific job
gha push -j test --watch
```

### Workflow Validation

```bash
# Validate workflow syntax
gha --validate

# Strict validation (more rigorous checks)
gha --validate --strict
```

### Working Directory and Workflow Path

```bash
# Run from different directory
gha push --directory /path/to/repo

# Use workflows from different path
gha push --workflows /path/to/workflows

# Disable recursive workflow search
gha push --no-recurse
```

## Commands Reference

### Main Command

```text
gha [event name] [flags]
```

If no event name is provided, GHA defaults to "push" or uses the only available event if there's just one.

#### Core Flags

| Flag | Short | Description | Example |
|------|-------|-------------|---------|
| `--job` | `-j` | Run specific job ID | `gha push -j build` |
| `--list` | `-l` | List available workflows | `gha --list` |
| `--graph` | `-g` | Show workflow dependency graph | `gha --graph` |
| `--watch` | `-w` | Watch files and re-run on changes | `gha push --watch` |
| `--validate` | | Validate workflow files | `gha --validate` |
| `--strict` | | Use strict workflow validation | `gha --validate --strict` |

#### Input and Configuration Flags

| Flag | Short | Description | Example |
|------|-------|-------------|---------|
| `--secret` | `-s` | Set secret values | `gha push -s TOKEN=abc123` |
| `--var` | | Set variable values | `gha push --var ENV=prod` |
| `--env` | | Set environment variables | `gha push --env DEBUG=true` |
| `--input` | | Set action inputs | `gha push --input version=1.0` |
| `--secret-file` | | Load secrets from file | `gha push --secret-file .secrets` |
| `--var-file` | | Load variables from file | `gha push --var-file .vars` |
| `--env-file` | | Load environment from file | `gha push --env-file .env` |
| `--input-file` | | Load inputs from file | `gha push --input-file .input` |

#### Workflow and Directory Flags

| Flag | Short | Description | Example |
|------|-------|-------------|---------|
| `--workflows` | `-W` | Path to workflow files | `gha push -W ./custom/workflows` |
| `--directory` | `-C` | Working directory | `gha push -C /path/to/repo` |
| `--no-recurse` | | Don't search subdirectories for workflows | `gha push --no-recurse` |
| `--eventpath` | `-e` | Path to event JSON file | `gha push -e event.json` |

#### Container and Platform Flags

| Flag | Short | Description | Example |
|------|-------|-------------|---------|
| `--platform` | `-P` | Custom platform images | `gha push -P ubuntu-20.04=custom:latest` |
| `--pull` | `-p` | Force pull Docker images (default: true) | `gha push --pull` |
| `--rebuild` | | Force rebuild local actions (default: true) | `gha push --rebuild` |
| `--bind` | `-b` | Bind working directory instead of copy | `gha push --bind` |
| `--reuse` | `-r` | Reuse containers between runs | `gha push --reuse` |
| `--privileged` | | Run containers in privileged mode | `gha push --privileged` |
| `--container-architecture` | | Set container architecture | `gha push --container-architecture linux/amd64` |
| `--container-options` | | Custom container options | `gha push --container-options "--memory=2g"` |

#### Output and Logging Flags

| Flag | Short | Description | Example |
|------|-------|-------------|---------|
| `--verbose` | `-v` | Verbose output | `gha push --verbose` |
| `--quiet` | `-q` | Disable step output logging | `gha push --quiet` |
| `--json` | | Output logs in JSON format | `gha push --json` |
| `--log-prefix-job-id` | | Use job ID in log prefix | `gha push --log-prefix-job-id` |
| `--dryrun` | `-n` | Validate without running containers | `gha push --dryrun` |

#### Advanced Flags

| Flag | Description | Example |
|------|-------------|---------|
| `--actor` | User that triggered the event (default: "Leapfrog-DevOps/gha") | `gha push --actor username` |
| `--defaultbranch` | Name of the default branch | `gha push --defaultbranch main` |
| `--remote-name` | Git remote name for repo URL | `gha push --remote-name upstream` |
| `--github-instance` | GitHub instance URL (for Enterprise) | `gha push --github-instance github.company.com` |
| `--use-gitignore` | Respect .gitignore when copying files | `gha push --use-gitignore` |
| `--matrix` | Include specific matrix combinations | `gha push --matrix os:ubuntu-20.04` |
| `--artifact-server-path` | Enable artifact server with storage path | `gha push --artifact-server-path ./artifacts` |
| `--network` | Docker network name | `gha push --network custom-network` |

### Actions Commands

The `gha actions` command provides GitHub API integration for managing workflows, runs, and logs.

#### Available Subcommands

```bash
# List all workflows in the repository
gha actions list

# List workflow runs
gha actions runs [workflow-id]

# Show jobs for a workflow run
gha actions jobs <run-id>

# Show logs for a workflow run
gha actions logs <run-id>

# Show detailed run information
gha actions show <run-id>

# Watch runs in real-time
gha actions watch [workflow-id]

# Rerun a workflow
gha actions rerun <run-id>

# Trigger a workflow manually
gha actions trigger <workflow-name>
```

#### Flag-Style Usage (Alternative)

```bash
# List workflows using flags
gha actions --list

# List runs using flags
gha actions --runs
gha actions --runs-for workflow-id

# Show run details using flags
gha actions --show run-id
gha actions --jobs run-id
gha actions --logs run-id

# Watch and manage using flags
gha actions --watch
gha actions --rerun run-id
gha actions --trigger workflow-name
```

#### Common Options

```bash
# Limit number of runs displayed
gha actions runs --limit 20

# Filter by status
gha actions runs --status completed

# Filter by branch
gha actions runs --branch main

# Show logs for specific job
gha actions logs run-id --job job-name

# Show raw logs without formatting
gha actions logs run-id --raw

# Rerun only failed jobs
gha actions rerun run-id --failed-only

# Trigger on specific branch
gha actions trigger workflow-name --ref feature-branch
```

#### Authentication

The actions commands require GitHub authentication. Set your token via:

```bash
# Environment variable
export GITHUB_TOKEN=ghp_xxxxxxxxxxxx

# Or use gh CLI authentication
gh auth login

# Or pass token directly (not recommended for security)
gha actions list -s GITHUB_TOKEN=ghp_xxxxxxxxxxxx
```

> **Note**: For more authentication options, see the [Configuration](#configuration) section.

### OIDC Commands

GHA includes an OIDC (OpenID Connect) server for testing cloud provider integrations locally. This is particularly useful for testing AWS authentication in GitHub Actions.

#### Server Management

```bash
# Start OIDC server with ngrok forwarding
gha oidc start

# Start with custom domain (command line)
gha oidc start --domain my-custom-domain.ngrok.io

# Or configure domain in .gharc for persistent use
echo "--domain my-custom-domain.ngrok.io" >> .gharc
gha oidc start

# Check server status
gha oidc status

# Stop OIDC server
gha oidc stop

# Restart server (keeps ngrok running)
gha oidc restart
```

#### Cloud Provider Setup

##### AWS OIDC Integration

```bash
# Setup AWS OIDC identity provider and IAM role
gha oidc setup --provider aws --role-name gha-test-role

# This creates:
# - OIDC Identity Provider in AWS IAM
# - IAM Role with trust policy for your OIDC provider
# - Outputs role ARN for use in workflows
```

Example workflow using AWS OIDC:

```yaml
name: Test AWS OIDC
on: push

jobs:
  test-aws:
    runs-on: ubuntu-latest
    permissions:
      id-token: write
      contents: read
    steps:
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::123456789012:role/gha-test-role
          aws-region: us-east-1
      - run: aws sts get-caller-identity
```

**Note**: Currently only AWS provider is supported for OIDC setup. Azure and GCP support may be added in future versions.

#### Cleanup

```bash
# Remove AWS OIDC setup
gha oidc cleanup --provider aws --role-name gha-test-role

# This removes:
# - IAM Role
# - OIDC Identity Provider
```

#### OIDC Environment Variables

When the OIDC server is running, GHA automatically sets these environment variables in your workflows:

- `ACTIONS_ID_TOKEN_REQUEST_URL` - OIDC token endpoint
- `ACTIONS_ID_TOKEN_REQUEST_TOKEN` - Request token for authentication
- `GITHUB_ACTIONS=true` - Indicates GitHub Actions environment

#### Prerequisites for OIDC

- **ngrok**: Required for exposing local OIDC server to cloud providers

  ```bash
  # Download from https://ngrok.com/download

  # Authenticate ngrok (free account required)
  ngrok authtoken YOUR_AUTHTOKEN
  ```

- **AWS CLI**: Required for AWS OIDC setup commands

  ```bash
  # AWS CLI
  aws configure
  ```

> **Tip**: For troubleshooting OIDC issues, see the [Troubleshooting](#troubleshooting) section.

### Utility Commands

#### Workflow Visualization

```bash
# List all available workflows and jobs
gha --list

# Show workflow dependency graph
gha --graph

# List workflows for specific event
gha pull_request --list

# Show graph for specific event
gha push --graph
```

Example output of `gha --list`:

```text
Job ID       Job name    Stage  Workflow name  Workflow file    Events
build        build       0      CI             ci.yml           push,pull_request
test         test        1      CI             ci.yml           push,pull_request
deploy       deploy      2      CI             ci.yml           push
```

#### Workflow Validation Commands

```bash
# Basic validation
gha --validate

# Strict validation with enhanced checks
gha --validate --strict

# Validate specific workflow file
gha --validate --workflows ./custom/workflow.yml

# Dry run - validate without executing
gha push --dryrun
```

#### System Diagnostics

```bash
# Generate bug report with system information
gha --bug-report

# Show version information
gha --version

# List all available command options in JSON format
gha --list-options

# Generate manual page
gha --man-page
```

Example bug report output includes:

- GHA version and build info
- Operating system details
- Docker configuration
- Available socket locations
- Configuration files in use

#### Help and Documentation

```bash
# Show main help
gha --help

# Show help for actions commands
gha actions --help

# Show help for OIDC commands
gha oidc --help

# Show help for specific subcommand
gha actions --help
```

## Configuration

### Configuration Files (.gharc)

GHA supports configuration files to avoid repeating command-line flags. Configuration files are searched in this order:

1. `~/.config/gha/gharc` (XDG config directory)
2. `~/.gharc` (home directory)
3. `./.gharc` (current directory)

#### Configuration File Format

Configuration files contain command-line flags, one per line:

```bash
# .gharc example
--platform ubuntu-20.04=custom:latest
--secret-file .secrets
--env-file .env
--verbose
--pull
--actor myusername
```

**Note**: Flags with spaces in values should be quoted or use separate lines for each flag and value.

#### Common Configuration Examples

**Development Configuration:**

```bash
# .gharc for development
--verbose
--bind
--reuse
--env-file .env.local
--secret-file .secrets.local
```

**CI/Production Configuration:**

```bash
# .gharc for CI
--pull
--rebuild
--json
--quiet
--container-architecture linux/amd64
```

**Custom Platform Configuration:**

```bash
# .gharc with custom images
--platform ubuntu-latest=myregistry/ubuntu:latest
--platform node:16=myregistry/node:16-custom
--platform python:3.9=myregistry/python:3.9-custom
```

**OIDC Configuration:**

```bash
# .gharc with custom ngrok domain for OIDC
--domain my-custom-domain.ngrok.io
--artifact-server-path ./artifacts
```

### Environment Variables

#### Docker Configuration

| Variable | Description | Example |
|----------|-------------|---------|
| `DOCKER_HOST` | Docker daemon socket | `unix:///var/run/docker.sock` |
| `DOCKER_CERT_PATH` | Docker TLS certificate path | `/path/to/certs` |
| `DOCKER_TLS_VERIFY` | Enable Docker TLS verification | `1` |

#### GitHub Configuration

| Variable | Description | Example |
|----------|-------------|---------|
| `GITHUB_TOKEN` | GitHub personal access token | `ghp_xxxxxxxxxxxx` |
| `GITHUB_REPOSITORY` | Repository name | `owner/repo` |
| `GITHUB_ACTOR` | GitHub username | `username` |

#### GHA-Specific Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `GHA_CACHE_DIR` | Action cache directory | `~/.cache/gha` |
| `GHA_CONFIG_DIR` | Configuration directory | `~/.config/gha` |
| `GHA_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |

### Input Files

#### Secrets File (.secrets)

```bash
# .secrets file format
GITHUB_TOKEN=ghp_xxxxxxxxxxxx
API_KEY=secret123
DATABASE_PASSWORD=supersecret
```

#### Variables File (.vars)

```bash
# .vars file format
ENVIRONMENT=production
VERSION=1.0.0
REGION=us-east-1
```

#### Environment File (.env)

```bash
# .env file format
NODE_ENV=production
DEBUG=false
PORT=3000
```

#### Inputs File (.input)

```bash
# .input file format (for action inputs)
version=1.0.0
environment=staging
deploy=true
```

### YAML Configuration Files

GHA also supports YAML format for configuration files:

```yaml
# .secrets.yml
GITHUB_TOKEN: ghp_xxxxxxxxxxxx
API_KEY: secret123

# .vars.yml
ENVIRONMENT: production
VERSION: 1.0.0

# .env.yml
NODE_ENV: production
DEBUG: false
```

## Advanced Features

### Custom Platform Images

Override default runner images with custom ones:

```bash
# Use custom Ubuntu image
gha push --platform ubuntu-latest=myregistry/ubuntu:custom

# Use multiple custom platforms
gha push \
  --platform ubuntu-20.04=myregistry/ubuntu:20.04 \
  --platform node:16=myregistry/node:16-alpine \
  --platform python:3.9=myregistry/python:3.9-slim
```

Configuration file example:

```bash
# .gharc
--platform ubuntu-latest=myregistry/ubuntu:latest
--platform node:16=myregistry/node:16-custom
```

### Matrix Builds

Run specific matrix combinations:

```bash
# Run specific matrix combination
gha push --matrix os:ubuntu-20.04 --matrix node:16

# Run multiple matrix combinations
gha push --matrix os:ubuntu-20.04 --matrix os:macos-latest
```

Example workflow with matrix:

```yaml
strategy:
  matrix:
    os: [ubuntu-20.04, ubuntu-22.04, macos-latest]
    node: [14, 16, 18]
```

### Local Action Development

#### Using Local Actions

```bash
# Use local repository instead of remote
gha push --local-repository actions/checkout@v4=/path/to/local/checkout

# Multiple local repositories
gha push \
  --local-repository actions/checkout@v4=/path/to/checkout \
  --local-repository actions/setup-node@v3=/path/to/setup-node
```

#### Local Action Structure

```text
my-action/
├── action.yml          # Action metadata
├── Dockerfile         # For Docker actions
├── index.js          # For JavaScript actions
└── README.md
```

Example `action.yml`:

```yaml
name: 'My Local Action'
description: 'A custom local action'
inputs:
  message:
    description: 'Message to display'
    required: true
runs:
  using: 'docker'
  image: 'Dockerfile'
```

### Container Configuration

#### Advanced Container Options

```bash
# Custom container options
gha push --container-options "--memory=2g --cpus=2"

# Privileged mode (use with caution)
gha push --privileged

# Custom user namespace
gha push --userns host

# Add/drop kernel capabilities
gha push --container-cap-add SYS_PTRACE --container-cap-drop NET_RAW
```

#### Container Architecture

```bash
# Force specific architecture (useful for M1 Macs)
gha push --container-architecture linux/amd64

# Use ARM64 architecture
gha push --container-architecture linux/arm64
```

#### Custom Networks

```bash
# Use custom Docker network
gha push --network my-custom-network

# Use host networking
gha push --network host
```

### Artifact and Cache Servers

#### Artifact Server

```bash
# Enable artifact server with storage path
gha push --artifact-server-path ./artifacts

# Custom artifact server address and port
gha push \
  --artifact-server-path ./artifacts \
  --artifact-server-addr 0.0.0.0 \
  --artifact-server-port 8080
```

#### Cache Server

```bash
# Enable cache server
gha push --cache-server-path ./cache

# Custom cache server configuration
gha push \
  --cache-server-path ./cache \
  --cache-server-addr 0.0.0.0 \
  --cache-server-port 9000

# External cache server URL (behind proxy)
gha push --cache-server-external-url https://cache.example.com
```

### GitHub Enterprise Server

```bash
# Use GitHub Enterprise Server
gha push --github-instance github.company.com

# Replace GHE actions with GitHub.com actions
gha push \
  --github-instance github.company.com \
  --replace-ghe-action-with-github-com actions/checkout \
  --replace-ghe-action-token-with-github-com ghp_token_for_github_com
```

### Performance Optimization

#### Container Reuse

```bash
# Reuse containers between runs (faster subsequent runs)
gha push --reuse

# Bind working directory (faster file access)
gha push --bind
```

#### Parallel Jobs

```bash
# Limit concurrent jobs (default: number of CPUs)
gha push --concurrent-jobs 2

# Unlimited concurrent jobs
gha push --concurrent-jobs 0
```

#### Action Caching

```bash
# Use new action cache system
gha push --use-new-action-cache

# Offline mode (don't fetch actions if cached)
gha push --action-offline-mode

# Custom action cache path
gha push --action-cache-path /custom/cache/path
```

### Event Simulation

#### Custom Event Data

```bash
# Use custom event JSON file
gha push --eventpath event.json
```

Example `event.json`:

```json
{
  "ref": "refs/heads/main",
  "repository": {
    "name": "my-repo",
    "full_name": "owner/my-repo"
  },
  "pusher": {
    "name": "username"
  }
}
```

#### Branch and Actor Configuration

```bash
# Set custom actor (user who triggered event)
gha push --actor myusername

# Set default branch name
gha push --defaultbranch main

# Set git remote name
gha push --remote-name upstream
```

## Examples

### Complete Workflow Examples

#### Node.js Application

```yaml
# .github/workflows/nodejs.yml
name: Node.js CI

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        node-version: [16, 18, 20]

    steps:
    - uses: actions/checkout@v4

    - name: Use Node.js ${{ matrix.node-version }}
      uses: actions/setup-node@v4
      with:
        node-version: ${{ matrix.node-version }}
        cache: 'npm'

    - name: Install dependencies
      run: npm ci

    - name: Run tests
      run: npm test
      env:
        NODE_ENV: test

    - name: Upload coverage
      uses: actions/upload-artifact@v3
      with:
        name: coverage-${{ matrix.node-version }}
        path: coverage/

  build:
    runs-on: ubuntu-latest
    needs: test

    steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-node@v4
      with:
        node-version: '18'
        cache: 'npm'

    - run: npm ci
    - run: npm run build

    - name: Upload build artifacts
      uses: actions/upload-artifact@v3
      with:
        name: dist
        path: dist/
```

Run locally:

```bash
# Run all jobs
gha push

# Run specific matrix combination
gha push --matrix node-version:18

# Run with custom environment
gha push --env NODE_ENV=development
```

#### Docker Build and Push

```yaml
# .github/workflows/docker.yml
name: Docker Build

on:
  push:
    branches: [ main ]
    tags: [ 'v*' ]

jobs:
  build:
    runs-on: ubuntu-latest

    steps:
    - uses: actions/checkout@v4

    - name: Set up Docker Buildx
      uses: docker/setup-buildx-action@v3

    - name: Login to Docker Hub
      uses: docker/login-action@v3
      with:
        username: ${{ secrets.DOCKER_USERNAME }}
        password: ${{ secrets.DOCKER_PASSWORD }}

    - name: Extract metadata
      id: meta
      uses: docker/metadata-action@v5
      with:
        images: myapp/myimage

    - name: Build and push
      uses: docker/build-push-action@v5
      with:
        context: .
        push: true
        tags: ${{ steps.meta.outputs.tags }}
        labels: ${{ steps.meta.outputs.labels }}
```

Run locally:

```bash
# Run with Docker secrets
gha push -s DOCKER_USERNAME=myuser -s DOCKER_PASSWORD=mypass

# Or use secrets file
echo "DOCKER_USERNAME=myuser" > .secrets
echo "DOCKER_PASSWORD=mypass" >> .secrets
gha push --secret-file .secrets
```

#### Multi-Platform Build

```yaml
# .github/workflows/multi-platform.yml
name: Multi-Platform

on: [push]

jobs:
  build:
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]

    runs-on: ${{ matrix.os }}

    steps:
    - uses: actions/checkout@v4

    - name: Setup Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'

    - name: Build
      run: go build -v ./...

    - name: Test
      run: go test -v ./...
```

Run locally (Linux/macOS only):

```bash
# Run Ubuntu jobs only
gha push --matrix os:ubuntu-latest

# Use custom platform image
gha push --platform ubuntu-latest=ubuntu:22.04
```

### Common Use Cases

#### 1. Local Development Workflow

```bash
# Setup for development
echo "--bind" >> .gharc
echo "--reuse" >> .gharc
echo "--verbose" >> .gharc

# Run tests on file changes
gha push -j test --watch

# Quick validation
gha --validate --strict
```

#### 2. CI/CD Pipeline Testing

```bash
# Test full pipeline
gha push

# Test specific deployment job
gha push -j deploy --env ENVIRONMENT=staging

# Test with production secrets
gha push --secret-file .secrets.prod
```

#### 3. Action Development

```bash
# Test local action
gha push --local-repository ./my-action@v1=./local-action

# Test with different inputs
gha push --input version=1.0.0 --input debug=true
```

#### 4. Matrix Testing

```bash
# Test all matrix combinations
gha push

# Test specific combinations
gha push --matrix os:ubuntu-20.04 --matrix node:16
gha push --matrix python-version:3.9
```

#### 5. Debugging Workflows

```bash
# Verbose output with dry run
gha push --verbose --dryrun

# Debug with custom event
gha push --eventpath debug-event.json --verbose

# Test with minimal environment
gha push --env-file .env.minimal
```

## Troubleshooting

### Common Issues

#### Docker Connection Issues

**Problem**: `Cannot connect to the Docker daemon`

**Solutions**:

```bash
# Check if Docker is running
docker ps

# Check Docker socket permissions (Linux)
sudo chmod 666 /var/run/docker.sock

# Use custom Docker socket
gha push --container-daemon-socket unix:///custom/docker.sock

# Check Docker host environment variable
echo $DOCKER_HOST
```

**Problem**: `Permission denied while trying to connect to Docker`

**Solutions**:

```bash
# Add user to docker group (Linux)
sudo usermod -aG docker $USER
# Then logout and login again

# Or run with sudo (not recommended)
sudo gha push
```

#### Apple M1/M2 Mac Issues

**Problem**: Architecture compatibility issues on Apple Silicon

**Solutions**:

```bash
# Force AMD64 architecture
gha push --container-architecture linux/amd64

# Add to .gharc for permanent fix
echo "--container-architecture linux/amd64" >> ~/.gharc
```

#### Workflow Validation Errors

**Problem**: `Workflow file is invalid`

**Solutions**:

```bash
# Validate workflow syntax
gha --validate

# Use strict validation for detailed errors
gha --validate --strict

# Check specific workflow file
gha --validate --workflows .github/workflows/problematic.yml
```

#### Action Not Found Errors

**Problem**: `Action 'owner/action@version' not found`

**Solutions**:

```bash
# Force pull latest actions
gha push --pull

# Rebuild local actions
gha push --rebuild

# Use offline mode if action is cached
gha push --action-offline-mode

# Check action reference in workflow file
# Ensure correct owner/repo@version format
```

#### Network and Connectivity Issues

**Problem**: Cannot pull Docker images or actions

**Solutions**:

```bash
# Check internet connectivity
ping github.com

# Use custom network
gha push --network host

# Configure Docker proxy (if behind corporate firewall)
# Edit ~/.docker/config.json
```

#### Memory and Resource Issues

**Problem**: Out of memory or disk space errors

**Solutions**:

```bash
# Limit concurrent jobs
gha push --concurrent-jobs 1

# Clean up Docker resources
docker system prune -a

# Use custom container options
gha push --container-options "--memory=1g --cpus=1"

# Check available disk space
df -h
```

#### File Permission Issues

**Problem**: Permission denied accessing files in container

**Solutions**:

```bash
# Use bind mount instead of copy
gha push --bind

# Check file permissions
ls -la .github/workflows/

# Ensure files are readable
chmod +r .github/workflows/*
```

### Debugging and Diagnostics

#### System Information

```bash
# Generate comprehensive bug report
gha --bug-report

# Check GHA version
gha --version

# List configuration files in use
gha --bug-report | grep "Config files"
```

#### Verbose Logging

```bash
# Enable verbose output
gha push --verbose

# JSON formatted logs (for parsing)
gha push --json

# Disable step output (quiet mode)
gha push --quiet

# Show job ID in logs instead of full name
gha push --log-prefix-job-id
```

#### Dry Run Mode

```bash
# Validate without running containers
gha push --dryrun

# Combine with verbose for detailed validation
gha push --dryrun --verbose
```

#### Docker Debugging

```bash
# Check Docker system info
docker system info

# Check available Docker images
docker images

# Check running containers
docker ps

# Check Docker logs
docker logs <container_id>
```

#### Environment Debugging

```bash
# Check environment variables in workflow
gha push --env DEBUG=true

# Print all environment variables in step
# Add this step to your workflow:
- name: Debug Environment
  run: env | sort
```

### System Requirements

#### Minimum Requirements

- **Operating System**: Linux, macOS, or Windows with WSL2
- **Docker**: Version 20.10+
- **Memory**: 2GB RAM minimum, 4GB+ recommended
- **Disk Space**: 5GB+ for Docker images and action cache
- **Network**: Internet connection for pulling actions and images

#### Supported Platforms

- **Linux**: x86_64, ARM64
- **macOS**: Intel and Apple Silicon (M1/M2)
- **Windows**: WSL2 with Docker Desktop

#### Docker Setup

**Recommended Docker settings**:

- Memory: 4GB+
- CPU: 2+ cores
- Disk: 20GB+ for images

**Docker Desktop settings** (macOS/Windows):

```json
{
  "memoryMiB": 4096,
  "cpus": 2,
  "diskSizeMiB": 20480
}
```

### Performance Tips

#### Optimization Strategies

1. **Use Container Reuse**:

   ```bash
   gha push --reuse
   ```

2. **Enable Bind Mounting**:

   ```bash
   gha push --bind
   ```

3. **Limit Concurrent Jobs**:

   ```bash
   gha push --concurrent-jobs 2
   ```

4. **Use Action Caching**:

   ```bash
   gha push --action-offline-mode
   ```

5. **Custom Platform Images**:

   ```bash
   # Use smaller, optimized images
   gha push --platform ubuntu-latest=ubuntu:20.04
   ```

#### Monitoring Performance

```bash
# Monitor Docker resource usage
docker stats

# Monitor system resources
top
htop

# Check disk usage
du -sh ~/.cache/gha
du -sh /var/lib/docker
```

### Getting Help

#### Community Support

- **GitHub Issues**: [Report bugs and request features](https://github.com/Leapfrog-DevOps/gha/issues)
- **Discussions**: [Ask questions and share tips](https://github.com/Leapfrog-DevOps/gha/discussions)
- **Documentation**: [Official documentation](https://github.com/Leapfrog-DevOps/gha)

#### Before Reporting Issues

1. **Check existing issues**: Search for similar problems
2. **Generate bug report**: Run `gha --bug-report`
3. **Provide minimal reproduction**: Include workflow file and command used
4. **Include system information**: OS, Docker version, GHA version

#### Useful Information for Bug Reports

```bash
# System information
gha --bug-report

# Docker information
docker version
docker system info

# Workflow validation
gha --validate --strict

# Verbose output
gha push --verbose --dryrun
```

## Development

### Development Setup

#### Prerequisites

- **Go**: Version 1.24+ ([installation guide](https://golang.org/doc/install))
- **Docker**: Version 20.10+ ([installation guide](https://docs.docker.com/get-docker/))
- **Git**: For version control
- **Make**: For build automation (usually pre-installed on Linux/macOS)

#### Clone and Setup

```bash
# Clone the repository
git clone https://github.com/Leapfrog-DevOps/gha.git
cd gha

# Install dependencies
go mod tidy

# Verify setup
go version
docker --version
make --version
```

#### Development Workflow

```bash
# Run tests
make test

# Format code
make format

# Lint code
make lint

# Build binary
make build

# Install locally
make install

# Run all checks (format, lint, test)
make pr
```

#### Project Structure

```text
gha/
├── cmd/                    # Command implementations
│   ├── root.go            # Main command and flags
│   ├── actions.go         # GitHub Actions API commands
│   ├── oidc.go           # OIDC server commands
│   ├── list.go           # Workflow listing
│   └── graph.go          # Workflow visualization
├── pkg/                   # Core packages
│   ├── common/           # Shared utilities
│   ├── container/        # Docker integration
│   ├── model/            # Workflow models
│   ├── runner/           # Workflow execution
│   ├── gh/               # GitHub API client
│   └── artifacts/        # Artifact handling
├── main.go               # Application entry point
├── Makefile             # Build automation
├── go.mod               # Go module definition
└── VERSION              # Version information
```

#### Key Components

**Command Layer** (`cmd/`):

- `root.go`: Main CLI interface and flag definitions
- `actions.go`: GitHub API integration for workflow management
- `oidc.go`: OIDC server for cloud provider authentication

**Core Logic** (`pkg/`):

- `runner/`: Workflow execution engine
- `model/`: Workflow parsing and planning
- `container/`: Docker container management
- `common/`: Shared utilities and helpers

#### Environment Variables for Development

```bash
# Enable debug logging
export GHA_LOG_LEVEL=debug

# Use custom cache directory
export GHA_CACHE_DIR=/tmp/gha-cache

# Use custom Docker socket
export DOCKER_HOST=unix:///custom/docker.sock
```

#### IDE Configuration

**VS Code** (`.vscode/settings.json`):

```json
{
  "go.lintTool": "golangci-lint",
  "go.formatTool": "goimports",
  "go.testFlags": ["-v"],
  "go.buildFlags": ["-v"]
}
```

**GoLand/IntelliJ**:

- Enable Go modules support
- Configure golangci-lint as external tool
- Set up run configurations for tests### T
esting

#### Running Tests

```bash
# Run all tests
make test

# Run tests with verbose output
go test -v ./...

# Run tests for specific package
go test -v ./pkg/runner

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

#### Test Structure

```text
pkg/runner/testdata/
├── basic/                 # Basic workflow tests
├── matrix/               # Matrix build tests
├── secrets/              # Secret handling tests
├── local-action-js/      # JavaScript action tests
├── local-action-dockerfile/ # Docker action tests
└── uses-composite/       # Composite action tests
```

#### Writing Tests

**Unit Tests**:

```go
func TestWorkflowExecution(t *testing.T) {
    // Test implementation
    ctx := context.Background()
    config := &runner.Config{
        Workdir: "/tmp/test",
        EventName: "push",
    }

    // Run test
    err := runner.New(config).Run(ctx)
    assert.NoError(t, err)
}
```

**Integration Tests**:

```go
func TestDockerIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Integration test implementation
}
```

#### Test Data

Test workflows are located in `pkg/runner/testdata/`. Each test case includes:

- `push.yml`: Workflow file
- Expected behavior documentation
- Any required local actions or files

### Contributing

#### Contribution Process

1. **Fork the repository** on GitHub
2. **Create a feature branch**: `git checkout -b feature/my-feature`
3. **Make your changes** following the coding standards
4. **Add tests** for new functionality
5. **Run the test suite**: `make test`
6. **Format and lint**: `make format && make lint`
7. **Commit your changes**: `git commit -m "Add my feature"`
8. **Push to your fork**: `git push origin feature/my-feature`
9. **Create a Pull Request** on GitHub

#### Coding Standards

**Go Code Style**:

```bash
# Format code
go fmt ./...
make format

# Lint code
golangci-lint run
make lint

# Check for common issues
go vet ./...
```

**Commit Message Format**:

```text
type(scope): description

feat(runner): add support for composite actions
fix(docker): resolve container cleanup issue
docs(readme): update installation instructions
test(runner): add matrix build test cases
```

**Types**: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`

#### Code Review Guidelines

**For Contributors**:

- Keep PRs focused and small
- Include tests for new features
- Update documentation as needed
- Respond to review feedback promptly

**For Reviewers**:

- Check functionality and test coverage
- Verify documentation updates
- Ensure coding standards compliance
- Test changes locally when possible

#### Release Process

**Version Management**:

```bash
# Check current version
cat VERSION

# Update version (maintainers only)
make promote

# Create snapshot build
make snapshot
```

**Release Checklist**:

- [ ] All tests pass
- [ ] Documentation updated
- [ ] Version bumped in VERSION file
- [ ] Release notes prepared
- [ ] Binaries built and tested

### Architecture Notes

#### Workflow Execution Flow

1. **Parse**: Read and validate workflow files
2. **Plan**: Determine execution order and dependencies
3. **Prepare**: Pull/build required Docker images
4. **Execute**: Run jobs in containers with proper environment
5. **Cleanup**: Remove containers and temporary files

#### Key Design Principles

- **Docker-first**: All actions run in containers for consistency
- **GitHub compatibility**: Match GitHub Actions environment exactly
- **Local development**: Optimize for fast feedback loops
- **Extensibility**: Support custom platforms and local actions

#### Adding New Features

**New Command**:

1. Add command definition in `cmd/`
2. Implement business logic in `pkg/`
3. Add tests in appropriate test files
4. Update documentation

**New Flag**:

1. Add flag definition in `cmd/root.go`
2. Add to `Input` struct if needed
3. Implement functionality in relevant packages
4. Add tests and documentation

#### Performance Considerations

- **Container reuse**: Minimize container creation overhead
- **Image caching**: Leverage Docker layer caching
- **Parallel execution**: Run independent jobs concurrently
- **Resource limits**: Respect system resource constraints

### License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

### Acknowledgments

- GitHub Actions team for the excellent CI/CD platform
- Docker community for containerization technology
- Go community for the robust programming language
