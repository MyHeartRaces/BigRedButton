package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"time"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	"github.com/MyHeartRaces/BigRedButton/internal/profile"
	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
	"github.com/MyHeartRaces/BigRedButton/internal/status"
)

type Options struct {
	Addr    string
	OpenURL bool
	Logger  *log.Logger
}

type app struct {
	configDir string
	cliPath   string
	logger    *log.Logger
}

type guiState struct {
	ProfilePath     string `json:"profile_path,omitempty"`
	EndpointIP      string `json:"endpoint_ip,omitempty"`
	WSTunnelBinary  string `json:"wstunnel_binary,omitempty"`
	LastCommand     string `json:"last_command,omitempty"`
	LastCommandTime string `json:"last_command_time,omitempty"`
	LastOutput      string `json:"last_output,omitempty"`
}

type statusResponse struct {
	OS        string           `json:"os"`
	GUI       guiState         `json:"gui"`
	Runtime   status.Snapshot  `json:"runtime"`
	Profile   *profile.Summary `json:"profile,omitempty"`
	ProfileOK bool             `json:"profile_ok"`
	Error     string           `json:"error,omitempty"`
}

type actionRequest struct {
	EndpointIP     string `json:"endpoint_ip"`
	WSTunnelBinary string `json:"wstunnel_binary"`
}

type actionResponse struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

func Run(ctx context.Context, options Options) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve user config directory: %w", err)
	}
	configDir = filepath.Join(configDir, "Big Red Button")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	logger := options.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "big-red-button-gui: ", log.LstdFlags)
	}
	cliPath, err := findCLI()
	if err != nil {
		logger.Printf("CLI lookup failed: %v", err)
	}

	a := &app{
		configDir: configDir,
		cliPath:   cliPath,
		logger:    logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.index)
	mux.HandleFunc("/api/status", a.status)
	mux.HandleFunc("/api/profile", a.profile)
	mux.HandleFunc("/api/connect", a.connect)
	mux.HandleFunc("/api/disconnect", a.disconnect)

	addr := strings.TrimSpace(options.Addr)
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	url := "http://" + listener.Addr().String()
	if options.OpenURL || options.Addr == "" {
		if err := openBrowser(url); err != nil {
			logger.Printf("open browser: %v", err)
			logger.Printf("open manually: %s", url)
		}
	}
	logger.Printf("serving %s", url)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-serverErr
		return ctx.Err()
	case err := <-serverErr:
		return err
	}
}

func (a *app) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, indexHTML)
}

func (a *app) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, a.statusPayload(r.Context()))
}

func (a *app) profile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "parse profile upload: " + err.Error()})
		return
	}
	file, _, err := r.FormFile("profile")
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "profile file is required"})
		return
	}
	defer file.Close()

	payload, err := io.ReadAll(io.LimitReader(file, 4<<20))
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "read profile: " + err.Error()})
		return
	}
	config, err := profile.ParseV7(payload)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "profile invalid: " + err.Error()})
		return
	}

	profileDir := filepath.Join(a.configDir, "profiles")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, actionResponse{Error: "create profile directory: " + err.Error()})
		return
	}
	path := filepath.Join(profileDir, "current.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, actionResponse{Error: "save profile: " + err.Error()})
		return
	}
	_ = os.Chmod(path, 0o600)

	state := a.loadState()
	state.ProfilePath = path
	state.LastCommand = "profile"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = "profile saved: " + config.Fingerprint()
	_ = a.saveState(state)

	writeJSON(w, a.statusPayload(r.Context()))
}

func (a *app) connect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	request, err := decodeAction(r.Body)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: err.Error()})
		return
	}
	if stdruntime.GOOS != "linux" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "real connect is implemented only on Linux"})
		return
	}
	state := a.loadState()
	if strings.TrimSpace(request.EndpointIP) != "" {
		state.EndpointIP = strings.TrimSpace(request.EndpointIP)
	}
	if strings.TrimSpace(request.WSTunnelBinary) != "" {
		state.WSTunnelBinary = strings.TrimSpace(request.WSTunnelBinary)
	}
	if strings.TrimSpace(state.ProfilePath) == "" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "upload a profile first"})
		return
	}
	if strings.TrimSpace(state.EndpointIP) == "" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "endpoint IP is required"})
		return
	}

	args := []string{"linux-connect", "-yes", "-endpoint-ip", state.EndpointIP}
	if strings.TrimSpace(state.WSTunnelBinary) != "" {
		args = append(args, "-wstunnel-binary", state.WSTunnelBinary)
	}
	args = append(args, state.ProfilePath)
	response := a.runCLI(r.Context(), "connect", args)
	state.LastCommand = "connect"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = response.Output
	_ = a.saveState(state)
	writeJSON(w, response)
}

func (a *app) disconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if stdruntime.GOOS != "linux" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "real disconnect is implemented only on Linux"})
		return
	}
	state := a.loadState()
	if strings.TrimSpace(state.ProfilePath) == "" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "upload a profile first"})
		return
	}

	response := a.runCLI(r.Context(), "disconnect", []string{"linux-disconnect", "-yes", state.ProfilePath})
	state.LastCommand = "disconnect"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = response.Output
	_ = a.saveState(state)
	writeJSON(w, response)
}

func (a *app) statusPayload(ctx context.Context) statusResponse {
	state := a.loadState()
	response := statusResponse{
		OS:      stdruntime.GOOS,
		GUI:     state,
		Runtime: status.FromStore(ctx, truntime.Store{Root: planner.DefaultRuntimeRoot}),
	}
	if strings.TrimSpace(state.ProfilePath) == "" {
		return response
	}
	config, err := profile.LoadFile(state.ProfilePath)
	if err != nil {
		response.Error = "profile invalid: " + err.Error()
		return response
	}
	summary := config.Summary()
	response.Profile = &summary
	response.ProfileOK = true
	return response
}

func (a *app) runCLI(ctx context.Context, action string, args []string) actionResponse {
	if strings.TrimSpace(a.cliPath) == "" {
		return actionResponse{Error: "big-red-button CLI was not found"}
	}
	command := append([]string{a.cliPath}, args...)
	if stdruntime.GOOS == "linux" {
		if pkexec, err := exec.LookPath("pkexec"); err == nil {
			command = append([]string{pkexec}, command...)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	result := strings.TrimSpace(output.String())
	if err != nil {
		if result == "" {
			result = err.Error()
		}
		return actionResponse{OK: false, Output: result, Error: action + " failed"}
	}
	if result == "" {
		result = action + " completed"
	}
	return actionResponse{OK: true, Output: result}
}

func (a *app) loadState() guiState {
	path := a.statePath()
	payload, err := os.ReadFile(path)
	if err != nil {
		return guiState{}
	}
	var state guiState
	if err := json.Unmarshal(payload, &state); err != nil {
		return guiState{}
	}
	return state
}

func (a *app) saveState(state guiState) error {
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(a.configDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(a.statePath(), payload, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(a.statePath(), 0o600)
	return nil
}

func (a *app) statePath() string {
	return filepath.Join(a.configDir, "gui-state.json")
}

func decodeAction(reader io.Reader) (actionRequest, error) {
	var request actionRequest
	if err := json.NewDecoder(reader).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		return request, fmt.Errorf("decode request: %w", err)
	}
	request.EndpointIP = strings.TrimSpace(request.EndpointIP)
	request.WSTunnelBinary = strings.TrimSpace(request.WSTunnelBinary)
	return request, nil
}

func findCLI() (string, error) {
	if value := strings.TrimSpace(os.Getenv("BRB_CLI_PATH")); value != "" {
		return value, nil
	}

	executable, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(executable)
		candidates := []string{
			filepath.Join(dir, cliName()),
			filepath.Join(dir, "..", "Resources", cliName()),
			filepath.Join(dir, "..", "..", "Resources", cliName()),
		}
		for _, candidate := range candidates {
			if fileExists(candidate) {
				return filepath.Clean(candidate), nil
			}
		}
	}

	path, err := exec.LookPath(cliName())
	if err != nil {
		return "", err
	}
	return path, nil
}

func cliName() string {
	if stdruntime.GOOS == "windows" {
		return "big-red-button.exe"
	}
	return "big-red-button"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func openBrowser(url string) error {
	switch stdruntime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	writeJSONStatus(w, http.StatusOK, value)
}

func writeJSONStatus(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSONStatus(w, http.StatusMethodNotAllowed, actionResponse{Error: "method not allowed"})
}
