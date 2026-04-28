package desktop

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"strconv"
	"strings"
	"time"

	"github.com/MyHeartRaces/BigRedButton/internal/buildinfo"
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

var (
	desktopGOOS     = stdruntime.GOOS
	desktopGeteuid  = os.Geteuid
	desktopLookPath = exec.LookPath
)

type guiState struct {
	ProfilePath     string `json:"profile_path,omitempty"`
	EndpointIP      string `json:"endpoint_ip,omitempty"`
	WSTunnelBinary  string `json:"wstunnel_binary,omitempty"`
	IsolatedSession string `json:"isolated_session,omitempty"`
	IsolatedCommand string `json:"isolated_command,omitempty"`
	LastCommand     string `json:"last_command,omitempty"`
	LastCommandTime string `json:"last_command_time,omitempty"`
	LastOutput      string `json:"last_output,omitempty"`
}

type statusResponse struct {
	Version          buildinfo.Info                   `json:"version"`
	OS               string                           `json:"os"`
	CLIPath          string                           `json:"cli_path,omitempty"`
	PrivilegeHelper  string                           `json:"privilege_helper,omitempty"`
	GUI              guiState                         `json:"gui"`
	Runtime          status.Snapshot                  `json:"runtime"`
	Isolated         *status.Snapshot                 `json:"isolated,omitempty"`
	IsolatedSessions []status.IsolatedSessionSnapshot `json:"isolated_sessions,omitempty"`
	Profile          *profile.Summary                 `json:"profile,omitempty"`
	ProfileOK        bool                             `json:"profile_ok"`
	Error            string                           `json:"error,omitempty"`
}

type actionRequest struct {
	EndpointIP     string `json:"endpoint_ip"`
	WSTunnelBinary string `json:"wstunnel_binary"`
	SessionID      string `json:"session_id"`
	AppCommand     string `json:"app_command"`
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
	mux.HandleFunc("/api/diagnostics", a.diagnostics)
	mux.HandleFunc("/api/preflight", a.preflight)
	mux.HandleFunc("/api/isolated/preflight", a.isolatedPreflight)
	mux.HandleFunc("/api/isolated/start", a.isolatedStart)
	mux.HandleFunc("/api/isolated/stop", a.isolatedStop)
	mux.HandleFunc("/api/isolated/cleanup", a.isolatedCleanup)
	mux.HandleFunc("/api/isolated/recover", a.isolatedRecover)

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
	config, err := profile.ParseWGWS(payload)
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
	if desktopGOOS != "linux" {
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
	args, err := buildLinuxConnectArgs(state)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: err.Error()})
		return
	}

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
	if desktopGOOS != "linux" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "real disconnect is implemented only on Linux"})
		return
	}
	state := a.loadState()
	response := a.runCLI(r.Context(), "disconnect", []string{"linux-disconnect", "-yes"})
	state.LastCommand = "disconnect"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = response.Output
	_ = a.saveState(state)
	writeJSON(w, response)
}

func (a *app) diagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	state := a.loadState()
	response := a.runCLIUnprivileged(r.Context(), "diagnostics", buildDiagnosticsArgs(state))
	state.LastCommand = "diagnostics"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = response.Output
	_ = a.saveState(state)
	writeJSON(w, response)
}

func (a *app) preflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	request, err := decodeAction(r.Body)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: err.Error()})
		return
	}
	if desktopGOOS != "linux" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "Linux preflight is implemented only on Linux"})
		return
	}
	state := a.loadState()
	if strings.TrimSpace(request.EndpointIP) != "" {
		state.EndpointIP = strings.TrimSpace(request.EndpointIP)
	}
	if strings.TrimSpace(request.WSTunnelBinary) != "" {
		state.WSTunnelBinary = strings.TrimSpace(request.WSTunnelBinary)
	}
	args, err := buildLinuxPreflightArgs(state)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: err.Error()})
		return
	}
	response := a.runCLIUnprivileged(r.Context(), "preflight", args)
	state.LastCommand = "preflight"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = response.Output
	_ = a.saveState(state)
	writeJSON(w, response)
}

func (a *app) isolatedPreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	request, err := decodeAction(r.Body)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: err.Error()})
		return
	}
	if desktopGOOS != "linux" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "isolated app preflight is implemented only on Linux"})
		return
	}
	state := a.loadState()
	if strings.TrimSpace(request.WSTunnelBinary) != "" {
		state.WSTunnelBinary = strings.TrimSpace(request.WSTunnelBinary)
	}
	if strings.TrimSpace(request.SessionID) != "" {
		state.IsolatedSession = strings.TrimSpace(request.SessionID)
	}
	if strings.TrimSpace(request.AppCommand) != "" {
		state.IsolatedCommand = strings.TrimSpace(request.AppCommand)
	}
	args, err := buildLinuxIsolatedPreflightArgs(state)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: err.Error()})
		return
	}
	response := a.runCLIUnprivileged(r.Context(), "isolated app preflight", args)
	state.LastCommand = "isolated-preflight"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = response.Output
	_ = a.saveState(state)
	writeJSON(w, response)
}

func (a *app) isolatedStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	request, err := decodeAction(r.Body)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: err.Error()})
		return
	}
	if desktopGOOS != "linux" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "isolated app mode is implemented only on Linux"})
		return
	}
	state := a.loadState()
	if strings.TrimSpace(state.ProfilePath) == "" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "upload a profile first"})
		return
	}
	if strings.TrimSpace(request.WSTunnelBinary) != "" {
		state.WSTunnelBinary = strings.TrimSpace(request.WSTunnelBinary)
	}
	if strings.TrimSpace(request.SessionID) != "" {
		state.IsolatedSession = strings.TrimSpace(request.SessionID)
	}
	if strings.TrimSpace(state.IsolatedSession) == "" {
		sessionID, err := newUUID()
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, actionResponse{Error: "generate session UUID: " + err.Error()})
			return
		}
		state.IsolatedSession = sessionID
	}
	if strings.TrimSpace(request.AppCommand) != "" {
		state.IsolatedCommand = strings.TrimSpace(request.AppCommand)
	}
	command, err := splitCommandLine(state.IsolatedCommand)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: err.Error()})
		return
	}
	if len(command) == 0 {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "app command is required"})
		return
	}

	args := []string{"linux-isolated-app", "-yes", "-session-id", state.IsolatedSession}
	if strings.TrimSpace(state.WSTunnelBinary) != "" {
		args = append(args, "-wstunnel-binary", state.WSTunnelBinary)
	}
	for _, env := range desktopLaunchEnv() {
		args = append(args, "-app-env", env)
	}
	args = append(args, state.ProfilePath, "--")
	args = append(args, command...)
	response := a.runCLI(r.Context(), "isolated app start", args)
	state.LastCommand = "isolated-start"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = response.Output
	_ = a.saveState(state)
	writeJSON(w, response)
}

func (a *app) isolatedStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	request, err := decodeAction(r.Body)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: err.Error()})
		return
	}
	if desktopGOOS != "linux" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "isolated app mode is implemented only on Linux"})
		return
	}
	state := a.loadState()
	if strings.TrimSpace(request.SessionID) != "" {
		state.IsolatedSession = strings.TrimSpace(request.SessionID)
	}
	if strings.TrimSpace(state.IsolatedSession) == "" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "isolated session UUID is required"})
		return
	}
	response := a.runCLI(r.Context(), "isolated app stop", []string{"linux-stop-isolated-app", "-yes", "-session-id", state.IsolatedSession})
	state.LastCommand = "isolated-stop"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = response.Output
	state = clearIsolatedSessionOnSuccess(state, response)
	_ = a.saveState(state)
	writeJSON(w, response)
}

func (a *app) isolatedCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	request, err := decodeAction(r.Body)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: err.Error()})
		return
	}
	if desktopGOOS != "linux" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "isolated app cleanup is implemented only on Linux"})
		return
	}
	state := a.loadState()
	if strings.TrimSpace(request.SessionID) != "" {
		state.IsolatedSession = strings.TrimSpace(request.SessionID)
	}
	if strings.TrimSpace(state.IsolatedSession) == "" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "isolated session UUID is required"})
		return
	}
	response := a.runCLI(r.Context(), "isolated app cleanup", []string{"linux-cleanup-isolated-app", "-yes", "-session-id", state.IsolatedSession})
	state.LastCommand = "isolated-cleanup"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = response.Output
	state = clearIsolatedSessionOnSuccess(state, response)
	_ = a.saveState(state)
	writeJSON(w, response)
}

func (a *app) isolatedRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if desktopGOOS != "linux" {
		writeJSONStatus(w, http.StatusBadRequest, actionResponse{Error: "isolated app recovery is implemented only on Linux"})
		return
	}
	state := a.loadState()
	response := a.runCLI(r.Context(), "isolated app recovery", []string{"linux-recover-isolated-sessions", "-yes"})
	state.LastCommand = "isolated-recover"
	state.LastCommandTime = time.Now().Format(time.RFC3339)
	state.LastOutput = response.Output
	_ = a.saveState(state)
	writeJSON(w, response)
}

func (a *app) statusPayload(ctx context.Context) statusResponse {
	state := a.loadState()
	response := statusResponse{
		Version:         buildinfo.Current(),
		OS:              desktopGOOS,
		CLIPath:         a.cliPath,
		PrivilegeHelper: linuxPrivilegeHelperStatus(),
		GUI:             state,
		Runtime:         status.FromStore(ctx, truntime.Store{Root: planner.DefaultRuntimeRoot}),
	}
	if strings.TrimSpace(state.IsolatedSession) != "" {
		isolated := status.FromStore(ctx, truntime.Store{Root: filepath.Join(planner.DefaultRuntimeRoot, planner.DefaultIsolatedRuntimeSubdir, state.IsolatedSession)})
		response.Isolated = &isolated
	}
	if sessions, err := status.IsolatedSessions(ctx, planner.DefaultRuntimeRoot); err == nil {
		response.IsolatedSessions = sessions
	} else {
		response.Error = "list isolated sessions: " + err.Error()
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
	return a.runCLICommand(ctx, action, args, desktopGOOS == "linux")
}

func (a *app) runCLIUnprivileged(ctx context.Context, action string, args []string) actionResponse {
	return a.runCLICommand(ctx, action, args, false)
}

func (a *app) runCLICommand(ctx context.Context, action string, args []string, privileged bool) actionResponse {
	if strings.TrimSpace(a.cliPath) == "" {
		return actionResponse{Error: "big-red-button CLI was not found"}
	}
	command := append([]string{a.cliPath}, args...)
	if privileged && desktopGOOS == "linux" {
		var err error
		command, err = withLinuxPrivilegeHelper(command)
		if err != nil {
			return actionResponse{OK: false, Output: err.Error(), Error: action + " failed"}
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

func withLinuxPrivilegeHelper(command []string) ([]string, error) {
	if desktopGeteuid() == 0 {
		return command, nil
	}
	pkexec, err := desktopLookPath("pkexec")
	if err != nil {
		return nil, fmt.Errorf("pkexec was not found; install polkit or run big-red-button from a root shell: %w", err)
	}
	return append([]string{pkexec}, command...), nil
}

func linuxPrivilegeHelperStatus() string {
	if desktopGOOS != "linux" {
		return "not required on " + desktopGOOS
	}
	if desktopGeteuid() == 0 {
		return "not required while running as root"
	}
	pkexec, err := desktopLookPath("pkexec")
	if err != nil {
		return "missing pkexec"
	}
	return "pkexec: " + pkexec
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
	request.SessionID = strings.TrimSpace(request.SessionID)
	request.AppCommand = strings.TrimSpace(request.AppCommand)
	return request, nil
}

func buildLinuxConnectArgs(state guiState) ([]string, error) {
	profilePath := strings.TrimSpace(state.ProfilePath)
	if profilePath == "" {
		return nil, errors.New("upload a profile first")
	}
	endpointIP := strings.TrimSpace(state.EndpointIP)
	args := []string{"linux-connect", "-yes"}
	if endpointIP != "" {
		args = append(args, "-endpoint-ip", endpointIP)
	}
	if wstunnelBinary := strings.TrimSpace(state.WSTunnelBinary); wstunnelBinary != "" {
		args = append(args, "-wstunnel-binary", wstunnelBinary)
	}
	args = append(args, profilePath)
	return args, nil
}

func buildDiagnosticsArgs(state guiState) []string {
	args := []string{"diagnostics", "-runtime-root", planner.DefaultRuntimeRoot}
	if profilePath := strings.TrimSpace(state.ProfilePath); profilePath != "" {
		args = append(args, "-profile", profilePath)
	}
	return args
}

func buildLinuxPreflightArgs(state guiState) ([]string, error) {
	profilePath := strings.TrimSpace(state.ProfilePath)
	if profilePath == "" {
		return nil, errors.New("upload a profile first")
	}
	args := []string{"linux-preflight", "-discover-routes", "-require-pkexec"}
	if endpointIP := strings.TrimSpace(state.EndpointIP); endpointIP != "" {
		args = append(args, "-endpoint-ip", endpointIP)
	}
	if wstunnelBinary := strings.TrimSpace(state.WSTunnelBinary); wstunnelBinary != "" {
		args = append(args, "-wstunnel-binary", wstunnelBinary)
	}
	args = append(args, profilePath)
	return args, nil
}

func buildLinuxIsolatedPreflightArgs(state guiState) ([]string, error) {
	profilePath := strings.TrimSpace(state.ProfilePath)
	if profilePath == "" {
		return nil, errors.New("upload a profile first")
	}
	command, err := splitCommandLine(state.IsolatedCommand)
	if err != nil {
		return nil, err
	}
	if len(command) == 0 {
		return nil, errors.New("app command is required")
	}
	args := []string{"linux-preflight-isolated-app", "-require-pkexec"}
	if sessionID := strings.TrimSpace(state.IsolatedSession); sessionID != "" {
		args = append(args, "-session-id", sessionID)
	}
	if wstunnelBinary := strings.TrimSpace(state.WSTunnelBinary); wstunnelBinary != "" {
		args = append(args, "-wstunnel-binary", wstunnelBinary)
	}
	if desktopGOOS == "linux" {
		args = append(args, "-app-uid", strconv.Itoa(os.Getuid()), "-app-gid", strconv.Itoa(os.Getgid()))
	}
	for _, env := range desktopLaunchEnv() {
		args = append(args, "-app-env", env)
	}
	args = append(args, profilePath, "--")
	args = append(args, command...)
	return args, nil
}

func clearIsolatedSessionOnSuccess(state guiState, response actionResponse) guiState {
	if response.OK {
		state.IsolatedSession = ""
	}
	return state
}

func desktopLaunchEnv() []string {
	var env []string
	for _, key := range []string{"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "PULSE_SERVER", "PIPEWIRE_RUNTIME_DIR"} {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" && !strings.ContainsRune(value, '\x00') {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func splitCommandLine(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}
	for _, r := range value {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("app command has unterminated quote")
	}
	flush()
	return args, nil
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
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
