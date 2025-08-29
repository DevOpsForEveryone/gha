# OIDC Server for AWS Actions Configure Credentials

This document explains how to use gha's built-in OIDC server with AWS Actions Configure Credentials.

## Quick Start

1. **Start the OIDC server:**

   ```bash
   go run ./cmd/oidc-server/main.go -port 8080
   ```

2. **Run your workflow with gha:**

   ```bash
   gha -s ACTIONS_ID_TOKEN_REQUEST_TOKEN=your-token \
       -s ACTIONS_ID_TOKEN_REQUEST_URL=http://localhost:8080/token
   ```

## Response Format

The OIDC server returns the correct format expected by AWS Actions Configure Credentials:

```json
{
  "result": {
    "value": "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9..."
  }
}
```

## Environment Variables

- `ACTIONS_ID_TOKEN_REQUEST_TOKEN` - Bearer token for authentication
- `ACTIONS_ID_TOKEN_REQUEST_URL` - OIDC server endpoint URL

## Example Workflow

```yaml
name: AWS OIDC Test
on: push

jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions:
      id-token: write
      contents: read
    steps:
      - name: Configure AWS Credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::575108933710:role/tast-custom-oidc
          aws-region: us-east-1
          audience: sts.amazonaws.com
```

## Testing

Run the included test script:

```bash
./test-oidc.sh
```

## Server Options

- `-port` - Port to run the server on (default: 8080)
- `-issuer` - OIDC issuer URL (default: <http://localhost:PORT>)

## JWT Claims

The server generates JWTs with standard GitHub Actions claims:

- `iss` - Issuer URL
- `sub` - Subject (repo:owner/name:ref:refs/heads/branch)
- `aud` - Audience (from query parameter)
- `exp` - Expiration time
- `iat` - Issued at time
- Plus GitHub-specific claims (repository, actor, workflow, etc.)
