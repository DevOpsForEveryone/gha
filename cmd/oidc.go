package cmd

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/adrg/xdg"
	"github.com/golang-jwt/jwt/v5"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type OIDCStatus struct {
	Running   bool   `json:"running"`
	PID       int    `json:"pid,omitempty"`
	Port      int    `json:"port,omitempty"`
	NgrokURL  string `json:"ngrok_url,omitempty"`
	NgrokPID  int    `json:"ngrok_pid,omitempty"`
	StartTime string `json:"start_time,omitempty"`
	Password  string `json:"password,omitempty"`
}

func createOIDCCommand() *cobra.Command {
	oidcCmd := &cobra.Command{
		Use:   "oidc",
		Short: "Manage OIDC server for local GitHub Actions",
		Long: `Start, stop, and check status of OIDC server with ngrok forwarding

The OIDC server supports custom ngrok domains using the global --domain flag:
  gha oidc start --domain my-custom-domain.ngrok.io

Or configure the domain in .gharc for persistent use:
  echo "--domain my-custom-domain.ngrok.io" >> .gharc`,
	}

	// Hide global flags from help output except for domain
	hideGlobalFlags(oidcCmd)

	// Add domain flag locally so it shows in help
	oidcCmd.PersistentFlags().StringP("domain", "d", "", "Custom ngrok domain to use (e.g. myapp.ngrok.io)")

	// Add platform flag to prevent conflicts with .gharc config
	oidcCmd.PersistentFlags().StringArrayP("platform", "P", []string{}, "custom image to use per platform (ignored for OIDC commands)")

	oidcCmd.AddCommand(createOIDCStartCommand())
	oidcCmd.AddCommand(createOIDCStatusCommand())
	oidcCmd.AddCommand(createOIDCStopCommand())
	oidcCmd.AddCommand(createOIDCRestartCommand())
	oidcCmd.AddCommand(createOIDCSetupCommand())
	oidcCmd.AddCommand(createOIDCCleanupCommand())

	return oidcCmd
}

func createOIDCStartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start OIDC server and ngrok forwarding",
		Long: `Starts a local OIDC server on port 8080 and creates an ngrok tunnel for external access.
The server provides JWT tokens for GitHub Actions OIDC authentication.

Examples:
  gha oidc start                                    # Use random ngrok domain
  gha oidc start --domain my-domain.ngrok.io       # Use custom domain

Configure domain in .gharc for persistent use:
  echo "--domain my-domain.ngrok.io" >> .gharc`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get domain from local flag first, then fall back to global flag
			domain, _ := cmd.Flags().GetString("domain")
			if domain == "" {
				domain, _ = cmd.Root().PersistentFlags().GetString("domain")
			}
			return startOIDCServerWithDomain(domain)
		},
	}
	return cmd
}

func createOIDCStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show OIDC server status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showOIDCStatus()
		},
	}
}

func createOIDCStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop OIDC server and ngrok forwarding",
		RunE: func(cmd *cobra.Command, args []string) error {
			return stopOIDCServer()
		},
	}
}

func createOIDCRestartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart OIDC server (keeps ngrok running)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return restartOIDCServer()
		},
	}
}

func createOIDCSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Setup cloud provider OIDC integration",
		Long:  "Automatically configure cloud provider OIDC identity provider and IAM roles",
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, _ := cmd.Flags().GetString("provider")
			roleName, _ := cmd.Flags().GetString("role-name")
			policy, _ := cmd.Flags().GetString("policy")
			return setupOIDCProvider(provider, roleName, policy)
		},
	}
	cmd.Flags().StringP("provider", "p", "", "Cloud provider (aws)")
	cmd.Flags().StringP("role-name", "r", "gha-oidc-role", "IAM role name to create")
	cmd.Flags().StringP("policy", "", "ReadOnlyAccess", "AWS managed policy to attach (e.g., ReadOnlyAccess, PowerUserAccess)")
	cmd.MarkFlagRequired("provider")
	return cmd
}

func createOIDCCleanupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup cloud provider OIDC integration",
		Long:  "Remove cloud provider OIDC identity provider and IAM roles created by setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, _ := cmd.Flags().GetString("provider")
			roleName, _ := cmd.Flags().GetString("role-name")
			force, _ := cmd.Flags().GetBool("force")
			return cleanupOIDCProvider(provider, roleName, force)
		},
	}
	cmd.Flags().StringP("provider", "p", "", "Cloud provider (aws)")
	cmd.Flags().StringP("role-name", "r", "gha-oidc-role", "IAM role name to delete")
	cmd.Flags().BoolP("force", "f", false, "Skip confirmation prompts")
	cmd.MarkFlagRequired("provider")
	return cmd
}

func getOIDCStatusFile() (string, error) {
	return xdg.StateFile("gha/oidc-status.json")
}

func saveOIDCStatus(status *OIDCStatus) error {
	statusFile, err := getOIDCStatusFile()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statusFile, data, 0600)
}

func loadOIDCStatus() (*OIDCStatus, error) {
	statusFile, err := getOIDCStatusFile()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(statusFile); os.IsNotExist(err) {
		return &OIDCStatus{Running: false}, nil
	}

	data, err := os.ReadFile(statusFile)
	if err != nil {
		return nil, err
	}

	var status OIDCStatus
	err = json.Unmarshal(data, &status)
	if err != nil {
		return nil, err
	}

	// Verify processes are still running
	if status.Running {
		if !isProcessRunning(status.PID) {
			status.Running = false
			status.PID = 0
			status.Port = 0
			status.NgrokURL = ""
			status.NgrokPID = 0
			status.StartTime = ""
		} else if status.NgrokPID > 0 && !isProcessRunning(status.NgrokPID) {
			status.NgrokURL = ""
			status.NgrokPID = 0
		}
	}

	return &status, nil
}

func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

type OIDCServerImpl struct {
	privateKey    *rsa.PrivateKey
	publicKey     *rsa.PublicKey
	issuer        string
	port          int
	server        *http.Server
	expectedToken string
}

func NewOIDCServerImpl(port int, password string) (*OIDCServerImpl, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	issuer := fmt.Sprintf("http://localhost:%d", port)
	return &OIDCServerImpl{
		privateKey:    privateKey,
		publicKey:     &privateKey.PublicKey,
		issuer:        issuer,
		port:          port,
		expectedToken: password,
	}, nil
}

func (s *OIDCServerImpl) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate bearer token
	auth := r.Header.Get("Authorization")
	if auth == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}

	if len(auth) < 7 || auth[:7] != "Bearer " {
		http.Error(w, "Invalid authorization format", http.StatusUnauthorized)
		return
	}

	token := auth[7:]
	if s.expectedToken != "" && token != s.expectedToken {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Get audience from query parameters (support both ? and & formats)
	audience := r.URL.Query().Get("audience")
	if audience == "" {
		// Handle malformed URLs with & instead of ?
		if strings.Contains(r.URL.RawQuery, "audience=") {
			parts := strings.Split(r.URL.RawQuery, "audience=")
			if len(parts) > 1 {
				audience = strings.Split(parts[1], "&")[0]
			}
		}
	}
	if audience == "" {
		audience = "https://github.com/actions"
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": s.issuer,
		"sub": "github-actions",
		"aud": audience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"nbf": time.Now().Unix(),
	})
	jwtToken.Header["kid"] = "1"

	tokenString, err := jwtToken.SignedString(s.privateKey)
	if err != nil {
		http.Error(w, "Failed to sign token", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"value": tokenString,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *OIDCServerImpl) handleJWKS(w http.ResponseWriter, r *http.Request) {
	n := s.publicKey.N.Bytes()
	e := s.publicKey.E

	// Convert to base64url
	nBase64 := base64.RawURLEncoding.EncodeToString(n)
	eBytes := make([]byte, 4)
	eBytes[0] = byte(e >> 24)
	eBytes[1] = byte(e >> 16)
	eBytes[2] = byte(e >> 8)
	eBytes[3] = byte(e)
	// Remove leading zeros
	for len(eBytes) > 1 && eBytes[0] == 0 {
		eBytes = eBytes[1:]
	}
	eBase64 := base64.RawURLEncoding.EncodeToString(eBytes)

	response := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"use": "sig",
				"kid": "1",
				"n":   nBase64,
				"e":   eBase64,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *OIDCServerImpl) handleWellKnown(w http.ResponseWriter, r *http.Request) {
	config := map[string]interface{}{
		"issuer":                                s.issuer,
		"token_endpoint":                        s.issuer + "/token",
		"jwks_uri":                              s.issuer + "/.well-known/jwks",
		"subject_types_supported":               []string{"public"},
		"response_types_supported":              []string{"id_token"},
		"claims_supported":                      []string{"sub", "aud", "exp", "iat", "iss"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func (s *OIDCServerImpl) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", s.handleToken)
	// Handle malformed URLs with & instead of ?
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/token&") {
			// Rewrite the URL to proper format
			r.URL.Path = "/token"
			r.URL.RawQuery = strings.TrimPrefix(r.RequestURI, "/token&")
			s.handleToken(w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/.well-known/jwks", s.handleJWKS)
	mux.HandleFunc("/.well-known/openid-configuration", s.handleWellKnown)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s.server.ListenAndServe()
}

func (s *OIDCServerImpl) Stop() error {
	if s.server != nil {
		return s.server.Shutdown(context.Background())
	}
	return nil
}

var globalOIDCServer *OIDCServerImpl

func startOIDCServerProcess(port int) {
	password := os.Getenv("GHA_OIDC_PASSWORD")
	server, err := NewOIDCServerImpl(port, password)
	if err != nil {
		log.Errorf("Failed to create OIDC server: %v", err)
		return
	}

	globalOIDCServer = server
	log.Infof("Starting OIDC server on port %d", port)
	if err := server.Start(); err != nil && err != http.ErrServerClosed {
		log.Errorf("OIDC server error: %v", err)
	}
}

func startOIDCServer() error {
	return startOIDCServerWithDomain("")
}

func startOIDCServerWithDomain(domain string) error {
	status, err := loadOIDCStatus()
	if err != nil {
		return fmt.Errorf("failed to load status: %w", err)
	}

	if status.Running {
		fmt.Printf("OIDC server is already running (PID: %d, Port: %d)\n", status.PID, status.Port)
		if status.NgrokURL != "" {
			fmt.Printf("Ngrok URL: %s\n", status.NgrokURL)
		}
		return nil
	}

	// Check if running in server mode
	if os.Getenv("GHA_OIDC_MODE") == "server" {
		port, _ := strconv.Atoi(os.Getenv("GHA_PORT"))
		ngrokURL := os.Getenv("GHA_NGROK_URL")
		password := os.Getenv("GHA_OIDC_PASSWORD")
		server, err := NewOIDCServerImpl(port, password)
		if err != nil {
			return fmt.Errorf("failed to create OIDC server: %w", err)
		}
		server.issuer = ngrokURL
		return server.Start()
	}

	// Start OIDC server as background process
	port := 8080

	// Generate secure password
	passwordBytes := make([]byte, 32)
	if _, err := rand.Read(passwordBytes); err != nil {
		return fmt.Errorf("failed to generate password: %w", err)
	}
	password := fmt.Sprintf("%x", passwordBytes)

	// Start ngrok first - validate port
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port number: %d", port)
	}

	// Build ngrok command with optional domain
	var ngrokCmd *exec.Cmd
	if domain != "" {
		// Use custom domain
		ngrokCmd = exec.Command("ngrok", "http", strconv.Itoa(port), "--domain", domain)
		fmt.Printf("Starting ngrok with custom domain: %s\n", domain)
	} else {
		// Use default (random) domain
		ngrokCmd = exec.Command("ngrok", "http", strconv.Itoa(port))
		fmt.Println("Starting ngrok with random domain")
	}

	if err := ngrokCmd.Start(); err != nil {
		return fmt.Errorf("failed to start ngrok: %w", err)
	}

	// Wait for ngrok to establish tunnel
	time.Sleep(3 * time.Second)

	// Get ngrok URL
	ngrokURL, err := getNgrokURL()
	if err != nil {
		if killErr := ngrokCmd.Process.Kill(); killErr != nil {
			_ = killErr // explicitly ignore kill error
		}
		return fmt.Errorf("failed to get ngrok URL: %w", err)
	}

	// Now start server with ngrok URL - validate executable path
	if len(os.Args) == 0 || os.Args[0] == "" {
		return fmt.Errorf("invalid executable path")
	}
	serverCmd := exec.Command(os.Args[0], "oidc", "start")
	serverCmd.Env = append(os.Environ(), "GHA_OIDC_MODE=server", fmt.Sprintf("GHA_NGROK_URL=%s", ngrokURL), fmt.Sprintf("GHA_PORT=%d", port), fmt.Sprintf("GHA_OIDC_PASSWORD=%s", password))

	if err := serverCmd.Start(); err != nil {
		if killErr := ngrokCmd.Process.Kill(); killErr != nil {
			_ = killErr // explicitly ignore kill error
		}
		return fmt.Errorf("failed to start OIDC server: %w", err)
	}

	// Give server time to start
	time.Sleep(2 * time.Second)

	status = &OIDCStatus{
		Running:   true,
		PID:       serverCmd.Process.Pid,
		Port:      port,
		NgrokURL:  ngrokURL,
		NgrokPID:  ngrokCmd.Process.Pid,
		StartTime: time.Now().Format(time.RFC3339),
		Password:  password,
	}

	if err := saveOIDCStatus(status); err != nil {
		log.Warnf("Failed to save status: %v", err)
	}

	fmt.Printf("OIDC server started successfully!\n")
	fmt.Printf("PID: %d\n", status.PID)
	fmt.Printf("Port: %d\n", status.Port)
	if status.NgrokURL != "" {
		fmt.Printf("Ngrok URL: %s\n", status.NgrokURL)
	}
	fmt.Println("Server running in background. Use 'gha oidc stop' to stop.")

	return nil
}

func getNgrokURL() (string, error) {
	// Try to get ngrok URL from API
	cmd := exec.Command("curl", "-s", "http://localhost:4040/api/tunnels")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	var response struct {
		Tunnels []struct {
			PublicURL string `json:"public_url"`
		} `json:"tunnels"`
	}

	if err := json.Unmarshal(output, &response); err != nil {
		return "", err
	}

	if len(response.Tunnels) > 0 {
		return response.Tunnels[0].PublicURL, nil
	}

	return "", fmt.Errorf("no tunnels found")
}

func showOIDCStatus() error {
	status, err := loadOIDCStatus()
	if err != nil {
		return fmt.Errorf("failed to load status: %w", err)
	}

	if !status.Running {
		fmt.Println("OIDC server is not running")
		return nil
	}

	fmt.Printf("OIDC Server Status:\n")
	fmt.Printf("  Status: Running\n")
	fmt.Printf("  PID: %d\n", status.PID)
	fmt.Printf("  Port: %d\n", status.Port)
	fmt.Printf("  Started: %s\n", status.StartTime)

	if status.NgrokURL != "" {
		fmt.Printf("  Ngrok URL: %s\n", status.NgrokURL)
		fmt.Printf("  Ngrok PID: %d\n", status.NgrokPID)
	} else {
		fmt.Printf("  Ngrok: Not available\n")
	}

	return nil
}

func stopOIDCServer() error {
	status, err := loadOIDCStatus()
	if err != nil {
		return fmt.Errorf("failed to load status: %w", err)
	}

	if !status.Running {
		fmt.Println("OIDC server is not running")
		return nil
	}

	var errors []string

	// Stop OIDC server
	if status.PID > 0 {
		fmt.Printf("Attempting to stop OIDC server (PID: %d)\n", status.PID)
		if process, err := os.FindProcess(status.PID); err == nil {
			if err := process.Kill(); err != nil {
				errors = append(errors, fmt.Sprintf("Failed to kill OIDC server process: %v", err))
			} else {
				fmt.Printf("Stopped OIDC server (PID: %d)\n", status.PID)
			}
		} else {
			errors = append(errors, fmt.Sprintf("Failed to find OIDC server process: %v", err))
		}
	}

	// Stop ngrok
	if status.NgrokPID > 0 {
		fmt.Printf("Attempting to stop ngrok (PID: %d)\n", status.NgrokPID)
		if process, err := os.FindProcess(status.NgrokPID); err == nil {
			if err := process.Kill(); err != nil {
				errors = append(errors, fmt.Sprintf("Failed to kill ngrok process: %v", err))
			} else {
				fmt.Printf("Stopped ngrok (PID: %d)\n", status.NgrokPID)
			}
		} else {
			errors = append(errors, fmt.Sprintf("Failed to find ngrok process: %v", err))
		}
	}

	// Clear status
	status = &OIDCStatus{Running: false}
	if err := saveOIDCStatus(status); err != nil {
		log.Warnf("Failed to save status: %v", err)
	}

	if len(errors) > 0 {
		fmt.Printf("Some processes could not be stopped:\n")
		for _, errMsg := range errors {
			fmt.Printf("  - %s\n", errMsg)
		}
		fmt.Println("You may need to manually kill remaining processes")
	} else {
		fmt.Println("OIDC server and ngrok stopped successfully")
	}
	return nil
}

func restartOIDCServer() error {
	status, err := loadOIDCStatus()
	if err != nil {
		return fmt.Errorf("failed to load status: %w", err)
	}

	if !status.Running {
		fmt.Println("OIDC server is not running")
		return nil
	}

	// Stop only OIDC server, keep ngrok running
	if status.PID > 0 {
		fmt.Printf("Stopping OIDC server (PID: %d)\n", status.PID)
		if process, err := os.FindProcess(status.PID); err == nil {
			if killErr := process.Kill(); killErr != nil {
				_ = killErr // explicitly ignore kill error
			}
		}
	}

	// Get existing ngrok URL
	ngrokURL := status.NgrokURL
	if ngrokURL == "" {
		var err error
		ngrokURL, err = getNgrokURL()
		if err != nil {
			return fmt.Errorf("failed to get ngrok URL: %w", err)
		}
	}

	// Start new server with existing ngrok URL and password - validate executable path
	if len(os.Args) == 0 || os.Args[0] == "" {
		return fmt.Errorf("invalid executable path")
	}
	serverCmd := exec.Command(os.Args[0], "oidc", "start")
	serverCmd.Env = append(os.Environ(), "GHA_OIDC_MODE=server", fmt.Sprintf("GHA_NGROK_URL=%s", ngrokURL), fmt.Sprintf("GHA_PORT=%d", status.Port), fmt.Sprintf("GHA_OIDC_PASSWORD=%s", status.Password))

	if err := serverCmd.Start(); err != nil {
		return fmt.Errorf("failed to restart OIDC server: %w", err)
	}

	// Give server time to start
	time.Sleep(2 * time.Second)

	// Update status with new PID
	status.PID = serverCmd.Process.Pid
	if err := saveOIDCStatus(status); err != nil {
		log.Warnf("Failed to save status: %v", err)
	}

	// Get and display thumbprint
	thumbprint, err := getThumbprint(ngrokURL)
	if err != nil {
		log.Warnf("Failed to get thumbprint: %v", err)
	} else {
		fmt.Printf("\nOIDC server restarted successfully!\n")
		fmt.Printf("PID: %d\n", status.PID)
		fmt.Printf("Ngrok URL: %s\n", ngrokURL)
		fmt.Printf("Thumbprint: %s\n", thumbprint)
	}

	return nil
}

func getThumbprint(url string) (string, error) {
	// Extract hostname from URL
	if len(url) < 8 {
		return "", fmt.Errorf("invalid URL")
	}
	hostname := strings.TrimPrefix(url, "https://")
	hostname = strings.TrimPrefix(hostname, "http://")
	if idx := strings.Index(hostname, "/"); idx != -1 {
		hostname = hostname[:idx]
	}

	// Get root CA certificate thumbprint - validate hostname
	if hostname == "" || strings.ContainsAny(hostname, ";|&$`") {
		return "", fmt.Errorf("invalid hostname")
	}
	cmd := exec.Command("openssl", "s_client", "-servername", hostname, "-connect", hostname+":443", "-showcerts")
	cmd2 := exec.Command("openssl", "x509", "-fingerprint", "-sha1", "-noout")

	// Use pipe to connect commands safely
	cmd2.Stdin, _ = cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return "", err
	}
	fingerprint, err := cmd2.Output()
	if waitErr := cmd.Wait(); waitErr != nil && err == nil {
		err = waitErr
	}
	if err != nil {
		return "", err
	}

	// Extract thumbprint from fingerprint output
	fingerprintStr := strings.TrimSpace(string(fingerprint))
	if idx := strings.Index(fingerprintStr, "="); idx != -1 {
		thumbprint := strings.ReplaceAll(fingerprintStr[idx+1:], ":", "")
		return strings.ToUpper(thumbprint), nil
	}

	return "", fmt.Errorf("failed to parse fingerprint")
}

func setupOIDCProvider(provider, roleName, policy string) error {
	if provider != "aws" {
		return fmt.Errorf("unsupported provider: %s. Currently supported: aws", provider)
	}

	// Check if OIDC server is running
	status, err := loadOIDCStatus()
	if err != nil {
		return fmt.Errorf("failed to load OIDC status: %w", err)
	}

	if !status.Running || status.NgrokURL == "" {
		return fmt.Errorf("OIDC server is not running or ngrok URL not available. Please run 'gha oidc start' first")
	}

	fmt.Printf("Setting up AWS OIDC integration...\n")
	fmt.Printf("OIDC Provider URL: %s\n", status.NgrokURL)
	fmt.Printf("Role Name: %s\n", roleName)
	fmt.Printf("Policy: %s\n", policy)

	return setupAWSProvider(status.NgrokURL, roleName, policy)
}

func setupAWSProvider(ngrokURL, roleName, policy string) error {
	// Extract domain from ngrok URL
	domain := strings.TrimPrefix(ngrokURL, "https://")
	domain = strings.TrimPrefix(domain, "http://")

	// Get AWS account ID
	fmt.Println("Getting AWS account ID...")
	accountCmd := exec.Command("aws", "sts", "get-caller-identity", "--query", "Account", "--output", "text")
	accountOutput, err := accountCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get AWS account ID. Make sure AWS CLI is configured: %w", err)
	}
	accountID := strings.TrimSpace(string(accountOutput))
	fmt.Printf("AWS Account ID: %s\n", accountID)

	// Get SSL certificate thumbprint
	fmt.Println("Getting SSL certificate thumbprint...")
	thumbprint, err := getThumbprint(ngrokURL)
	if err != nil {
		return fmt.Errorf("failed to get SSL thumbprint: %w", err)
	}
	fmt.Printf("SSL Thumbprint: %s\n", thumbprint)

	// Create OIDC Identity Provider
	fmt.Println("Creating OIDC Identity Provider...")
	providerArn := fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", accountID, domain)

	createProviderCmd := exec.Command("aws", "iam", "create-open-id-connect-provider",
		"--url", ngrokURL,
		"--thumbprint-list", thumbprint,
		"--client-id-list", "sts.amazonaws.com")

	if output, err := createProviderCmd.CombinedOutput(); err != nil {
		// Check if provider already exists
		if strings.Contains(string(output), "already exists") {
			fmt.Println("OIDC Identity Provider already exists")
		} else {
			return fmt.Errorf("failed to create OIDC provider: %w\nOutput: %s", err, string(output))
		}
	} else {
		fmt.Println("OIDC Identity Provider created successfully")
	}

	// Create trust policy
	trustPolicy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "%s"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "%s:aud": "sts.amazonaws.com",
          "%s:sub": "github-actions"
        }
      }
    }
  ]
}`, providerArn, domain, domain)

	// Write trust policy to temp file
	trustPolicyFile := "/tmp/gha-trust-policy.json"
	if err := os.WriteFile(trustPolicyFile, []byte(trustPolicy), 0600); err != nil {
		return fmt.Errorf("failed to write trust policy: %w", err)
	}
	defer os.Remove(trustPolicyFile)

	// Create IAM Role
	fmt.Printf("Creating IAM Role: %s...\n", roleName)
	createRoleCmd := exec.Command("aws", "iam", "create-role",
		"--role-name", roleName,
		"--assume-role-policy-document", fmt.Sprintf("file://%s", trustPolicyFile))

	if output, err := createRoleCmd.CombinedOutput(); err != nil {
		// Check if role already exists
		if strings.Contains(string(output), "already exists") {
			fmt.Printf("IAM Role %s already exists\n", roleName)
		} else {
			return fmt.Errorf("failed to create IAM role: %w\nOutput: %s", err, string(output))
		}
	} else {
		fmt.Printf("IAM Role %s created successfully\n", roleName)
	}

	// Attach policy to role
	fmt.Printf("Attaching policy %s to role...\n", policy)
	policyArn := fmt.Sprintf("arn:aws:iam::aws:policy/%s", policy)
	attachPolicyCmd := exec.Command("aws", "iam", "attach-role-policy",
		"--role-name", roleName,
		"--policy-arn", policyArn)

	if output, err := attachPolicyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to attach policy: %w\nOutput: %s", err, string(output))
	}

	roleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)

	fmt.Println("\n✅ AWS OIDC Setup Complete!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("OIDC Provider: %s\n", ngrokURL)
	fmt.Printf("Provider ARN:  %s\n", providerArn)
	fmt.Printf("Role ARN:      %s\n", roleArn)
	fmt.Printf("Policy:        %s\n", policyArn)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("\nUse this in your workflow:")
	fmt.Printf(`      - name: Configure AWS Credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: %s
          aws-region: us-east-1
          audience: sts.amazonaws.com
`, roleArn)

	return nil
}

func cleanupOIDCProvider(provider, roleName string, force bool) error {
	if provider != "aws" {
		return fmt.Errorf("unsupported provider: %s. Currently supported: aws", provider)
	}

	fmt.Printf("Cleaning up AWS OIDC integration...\n")
	fmt.Printf("Role Name: %s\n", roleName)

	return cleanupAWSProvider(roleName, force)
}

func cleanupAWSProvider(roleName string, force bool) error {
	// Get AWS account ID
	fmt.Println("Getting AWS account ID...")
	accountCmd := exec.Command("aws", "sts", "get-caller-identity", "--query", "Account", "--output", "text")
	accountOutput, err := accountCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get AWS account ID. Make sure AWS CLI is configured: %w", err)
	}
	accountID := strings.TrimSpace(string(accountOutput))
	fmt.Printf("AWS Account ID: %s\n", accountID)

	// Try to get the OIDC provider URL from running server or ask user
	var domain string
	status, err := loadOIDCStatus()
	if err == nil && status.Running && status.NgrokURL != "" {
		domain = strings.TrimPrefix(status.NgrokURL, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		fmt.Printf("Found running OIDC server with domain: %s\n", domain)
	} else {
		fmt.Println("OIDC server not running. You'll need to provide the domain manually.")
		fmt.Print("Enter the OIDC provider domain (e.g., your-domain.ngrok-free.dev): ")
		var input string
		if _, err := fmt.Scanln(&input); err != nil {
			return fmt.Errorf("failed to read domain input: %w", err)
		}
		domain = strings.TrimSpace(input)
		if domain == "" {
			return fmt.Errorf("domain cannot be empty")
		}
	}

	providerArn := fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", accountID, domain)
	roleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)

	// Show what will be deleted
	fmt.Println("\n🗑️  Resources to be deleted:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("OIDC Provider: %s\n", providerArn)
	fmt.Printf("IAM Role:      %s\n", roleArn)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Confirmation prompt unless force flag is used
	if !force {
		fmt.Print("\nAre you sure you want to delete these resources? (y/N): ")
		var response string
		if _, err := fmt.Scanln(&response); err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cleanup cancelled.")
			return nil
		}
	}

	fmt.Println("\nStarting cleanup...")

	// Step 1: Detach policies from role
	fmt.Printf("Detaching policies from role %s...\n", roleName)

	// List attached policies
	listPoliciesCmd := exec.Command("aws", "iam", "list-attached-role-policies", "--role-name", roleName, "--output", "text", "--query", "AttachedPolicies[].PolicyArn")
	policiesOutput, err := listPoliciesCmd.Output()
	if err != nil {
		fmt.Printf("Warning: Could not list attached policies: %v\n", err)
	} else {
		policies := strings.Fields(string(policiesOutput))
		for _, policyArn := range policies {
			if policyArn != "" {
				fmt.Printf("  Detaching policy: %s\n", policyArn)
				detachCmd := exec.Command("aws", "iam", "detach-role-policy", "--role-name", roleName, "--policy-arn", policyArn)
				if output, err := detachCmd.CombinedOutput(); err != nil {
					fmt.Printf("  Warning: Failed to detach policy %s: %v\nOutput: %s\n", policyArn, err, string(output))
				}
			}
		}
	}

	// Step 2: Delete IAM Role
	fmt.Printf("Deleting IAM role %s...\n", roleName)
	deleteRoleCmd := exec.Command("aws", "iam", "delete-role", "--role-name", roleName)
	if output, err := deleteRoleCmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "NoSuchEntity") {
			fmt.Printf("  Role %s does not exist (already deleted)\n", roleName)
		} else {
			fmt.Printf("  Warning: Failed to delete role: %v\nOutput: %s\n", err, string(output))
		}
	} else {
		fmt.Printf("  ✅ Role %s deleted successfully\n", roleName)
	}

	// Step 3: Delete OIDC Identity Provider
	fmt.Printf("Deleting OIDC identity provider %s...\n", domain)
	deleteProviderCmd := exec.Command("aws", "iam", "delete-open-id-connect-provider", "--open-id-connect-provider-arn", providerArn)
	if output, err := deleteProviderCmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "NoSuchEntity") {
			fmt.Printf("  OIDC provider %s does not exist (already deleted)\n", domain)
		} else {
			fmt.Printf("  Warning: Failed to delete OIDC provider: %v\nOutput: %s\n", err, string(output))
		}
	} else {
		fmt.Printf("  ✅ OIDC provider %s deleted successfully\n", domain)
	}

	fmt.Println("\n✅ AWS OIDC Cleanup Complete!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("All AWS OIDC resources have been removed.")
	fmt.Println("You can now safely stop the OIDC server with: gha oidc stop")

	return nil
}

// hideGlobalFlags sets a custom help template that excludes global flags
func hideGlobalFlags(cmd *cobra.Command) {
	helpTemplate := `{{.Short}}{{if .Long}}

{{.Long}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
	cmd.SetHelpTemplate(helpTemplate)
}
