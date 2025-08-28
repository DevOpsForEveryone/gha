package cmd

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
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
}

func createOIDCCommand() *cobra.Command {
	oidcCmd := &cobra.Command{
		Use:   "oidc",
		Short: "Manage OIDC server for local GitHub Actions",
		Long:  "Start, stop, and check status of OIDC server with ngrok forwarding",
	}

	// Add platform flag to prevent conflicts with .gharc config
	oidcCmd.PersistentFlags().StringArrayP("platform", "P", []string{}, "custom image to use per platform (ignored for OIDC commands)")

	oidcCmd.AddCommand(createOIDCStartCommand())
	oidcCmd.AddCommand(createOIDCStatusCommand())
	oidcCmd.AddCommand(createOIDCStopCommand())

	return oidcCmd
}

func createOIDCStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start OIDC server and ngrok forwarding",
		RunE: func(cmd *cobra.Command, args []string) error {
			return startOIDCServer()
		},
	}
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

	return os.WriteFile(statusFile, data, 0644)
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
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	issuer     string
	port       int
	server     *http.Server
}

func NewOIDCServerImpl(port int) (*OIDCServerImpl, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	issuer := fmt.Sprintf("http://localhost:%d", port)
	return &OIDCServerImpl{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		issuer:     issuer,
		port:       port,
	}, nil
}

func (s *OIDCServerImpl) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": s.issuer,
		"sub": "github-actions",
		"aud": "https://github.com/actions",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"nbf": time.Now().Unix(),
	})

	tokenString, err := token.SignedString(s.privateKey)
	if err != nil {
		http.Error(w, "Failed to sign token", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"result": map[string]interface{}{
			"value": tokenString,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *OIDCServerImpl) handleJWKS(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"use": "sig",
				"kid": "1",
				"n":   "dummy-n-value",
				"e":   "AQAB",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *OIDCServerImpl) handleWellKnown(w http.ResponseWriter, r *http.Request) {
	config := map[string]interface{}{
		"issuer":                 s.issuer,
		"token_endpoint":         s.issuer + "/token",
		"jwks_uri":              s.issuer + "/.well-known/jwks.json",
		"subject_types_supported": []string{"public"},
		"response_types_supported": []string{"id_token"},
		"claims_supported": []string{"sub", "aud", "exp", "iat", "iss"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func (s *OIDCServerImpl) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", s.handleToken)
	mux.HandleFunc("/.well-known/jwks.json", s.handleJWKS)
	mux.HandleFunc("/.well-known/openid_configuration", s.handleWellKnown)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
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
	server, err := NewOIDCServerImpl(port)
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
		server, err := NewOIDCServerImpl(port)
		if err != nil {
			return fmt.Errorf("failed to create OIDC server: %w", err)
		}
		server.issuer = ngrokURL
		return server.Start()
	}

	// Start OIDC server as background process
	port := 8080
	
	// Start server first as subprocess
	serverCmd := exec.Command(os.Args[0], "oidc", "start")
	serverCmd.Env = append(os.Environ(), "GHA_OIDC_MODE=server", "GHA_NGROK_URL=http://localhost:8080", fmt.Sprintf("GHA_PORT=%d", port))
	
	if err := serverCmd.Start(); err != nil {
		return fmt.Errorf("failed to start OIDC server: %w", err)
	}

	// Give server time to start
	time.Sleep(2 * time.Second)
	
	// Now start ngrok to tunnel to the running server
	ngrokCmd := exec.Command("ngrok", "http", strconv.Itoa(port))
	if err := ngrokCmd.Start(); err != nil {
		serverCmd.Process.Kill()
		return fmt.Errorf("failed to start ngrok: %w", err)
	}
	
	// Wait for ngrok to establish tunnel
	time.Sleep(3 * time.Second)
	
	// Get ngrok URL
	ngrokURL, err := getNgrokURL()
	if err != nil {
		serverCmd.Process.Kill()
		ngrokCmd.Process.Kill()
		return fmt.Errorf("failed to get ngrok URL: %w", err)
	}

	status = &OIDCStatus{
		Running:   true,
		PID:       serverCmd.Process.Pid,
		Port:      port,
		NgrokURL:  ngrokURL,
		NgrokPID:  ngrokCmd.Process.Pid,
		StartTime: time.Now().Format(time.RFC3339),
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

	fmt.Printf("Debug: Status loaded - Running: %v, PID: %d\n", status.Running, status.PID)

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