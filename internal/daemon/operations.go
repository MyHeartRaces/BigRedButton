package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	"github.com/MyHeartRaces/BigRedButton/internal/profile"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args []string) (string, error)
}

type execCommandRunner struct{}

type service struct {
	runtimeRoot string
	cliPath     string
	runner      CommandRunner
}

func newService(options Options) service {
	runtimeRoot := strings.TrimSpace(options.RuntimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = planner.DefaultRuntimeRoot
	}
	runner := options.Runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	return service{
		runtimeRoot: runtimeRoot,
		cliPath:     strings.TrimSpace(options.CLIPath),
		runner:      runner,
	}
}

func (s service) Connect(ctx context.Context, request ConnectRequest) OperationResponse {
	if stdruntime.GOOS != "linux" {
		return operationError("connect is implemented only on Linux")
	}
	if _, err := profile.ParseWGWS(request.Profile); err != nil {
		return profileOperationError(err)
	}
	runtimeRoot := s.runtimeRootFor(request.Options.RuntimeRoot)
	request.Options.RuntimeRoot = runtimeRoot
	args := []string{"linux-connect", "-yes", "-json"}
	args = append(args, connectOptionArgs(request.Options)...)
	return s.runWithProfile(ctx, request.Profile, args, nil)
}

func (s service) Disconnect(ctx context.Context, request DisconnectRequest) OperationResponse {
	if stdruntime.GOOS != "linux" {
		return operationError("disconnect is implemented only on Linux")
	}
	runtimeRoot := s.runtimeRootFor(request.Options.RuntimeRoot)
	args := []string{"linux-disconnect", "-yes", "-json", "-runtime-root", runtimeRoot}
	if value := strings.TrimSpace(request.Options.WireGuardInterface); value != "" {
		args = append(args, "-wireguard-interface", value)
	}
	return s.runCLI(ctx, args)
}

func (s service) StartIsolated(ctx context.Context, request IsolatedStartRequest) OperationResponse {
	if stdruntime.GOOS != "linux" {
		return operationError("isolated app mode is implemented only on Linux")
	}
	if _, err := profile.ParseWGWS(request.Profile); err != nil {
		return profileOperationError(err)
	}
	request.Options.RuntimeRoot = s.runtimeRootFor(request.Options.RuntimeRoot)
	if len(request.Options.AppCommand) == 0 {
		return operationError("app command is required")
	}
	if strings.TrimSpace(request.Options.LaunchUID) == "" && strings.TrimSpace(request.Options.LaunchGID) == "" {
		if credentials, ok := peerCredentialsFromContext(ctx); ok && credentials.UID > 0 {
			request.Options.LaunchUID = strconv.Itoa(credentials.UID)
			request.Options.LaunchGID = strconv.Itoa(credentials.GID)
		}
	}
	args := []string{"linux-isolated-app", "-yes", "-json"}
	if request.CleanupOnExit != nil && !*request.CleanupOnExit {
		args = append(args, "-cleanup-on-exit=false")
	}
	args = append(args, isolatedOptionArgs(request.Options)...)
	return s.runWithProfile(ctx, request.Profile, args, append([]string{"--"}, request.Options.AppCommand...))
}

func (s service) StopIsolated(ctx context.Context, request IsolatedSessionRequest) OperationResponse {
	if stdruntime.GOOS != "linux" {
		return operationError("isolated app mode is implemented only on Linux")
	}
	sessionID := strings.TrimSpace(request.SessionID)
	if sessionID == "" {
		return operationError("session ID is required")
	}
	return s.runCLI(ctx, []string{
		"linux-stop-isolated-app",
		"-yes",
		"-json",
		"-runtime-root", s.runtimeRootFor(request.RuntimeRoot),
		"-session-id", sessionID,
	})
}

func (s service) CleanupIsolated(ctx context.Context, request IsolatedSessionRequest) OperationResponse {
	if stdruntime.GOOS != "linux" {
		return operationError("isolated app cleanup is implemented only on Linux")
	}
	sessionID := strings.TrimSpace(request.SessionID)
	if sessionID == "" {
		return operationError("session ID is required")
	}
	return s.runCLI(ctx, []string{
		"linux-cleanup-isolated-app",
		"-yes",
		"-json",
		"-runtime-root", s.runtimeRootFor(request.RuntimeRoot),
		"-session-id", sessionID,
	})
}

func (s service) RecoverIsolated(ctx context.Context, request IsolatedRecoverRequest) OperationResponse {
	if stdruntime.GOOS != "linux" {
		return operationError("isolated app recovery is implemented only on Linux")
	}
	args := []string{
		"linux-recover-isolated-sessions",
		"-yes",
		"-json",
		"-runtime-root", s.runtimeRootFor(request.RuntimeRoot),
	}
	if request.All {
		args = append(args, "-all")
	}
	if request.Startup {
		args = append(args, "-startup")
	}
	return s.runCLI(ctx, args)
}

func (s service) runWithProfile(ctx context.Context, payload []byte, argsBeforeProfile []string, argsAfterProfile []string) OperationResponse {
	path, cleanup, err := s.writeProfile(payload)
	if err != nil {
		return operationError(err.Error())
	}
	defer cleanup()
	args := append([]string(nil), argsBeforeProfile...)
	args = append(args, path)
	args = append(args, argsAfterProfile...)
	return s.runCLI(ctx, args)
}

func (s service) runCLI(ctx context.Context, args []string) OperationResponse {
	cliPath, err := s.resolveCLIPath()
	if err != nil {
		return operationError(err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	output, err := s.runner.Run(ctx, cliPath, args)
	output = strings.TrimSpace(output)
	if err != nil {
		if output == "" {
			output = err.Error()
		}
		return OperationResponse{OK: false, Output: output, Error: "operation failed"}
	}
	if output == "" {
		output = "operation completed"
	}
	return OperationResponse{OK: true, Output: output}
}

func (s service) writeProfile(payload []byte) (string, func(), error) {
	if len(bytes.TrimSpace(payload)) == 0 {
		return "", func() {}, fmt.Errorf("profile payload is required")
	}
	root := filepath.Join(s.runtimeRoot, "daemon-profiles")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", func() {}, fmt.Errorf("create daemon profile directory: %w", err)
	}
	file, err := os.CreateTemp(root, "profile-*.json")
	if err != nil {
		return "", func() {}, fmt.Errorf("create daemon profile file: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write daemon profile file: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close daemon profile file: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	return path, cleanup, nil
}

func (s service) resolveCLIPath() (string, error) {
	if s.cliPath != "" {
		return s.cliPath, nil
	}
	if value := strings.TrimSpace(os.Getenv("BRB_CLI_PATH")); value != "" {
		return value, nil
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		for _, candidate := range []string{
			filepath.Join(dir, cliName()),
			filepath.Join(dir, "..", cliName()),
		} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return filepath.Clean(candidate), nil
			}
		}
	}
	if path, err := exec.LookPath(cliName()); err == nil {
		return path, nil
	}
	if stdruntime.GOOS != "windows" {
		return "/usr/bin/big-red-button", nil
	}
	return "", fmt.Errorf("big-red-button CLI was not found")
}

func (s service) runtimeRootFor(value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return s.runtimeRoot
}

func (execCommandRunner) Run(ctx context.Context, name string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.String(), err
}

func connectOptionArgs(options planner.Options) []string {
	var args []string
	if len(options.EndpointIPs) > 0 {
		args = append(args, "-endpoint-ip", strings.Join(options.EndpointIPs, ","))
	}
	if value := strings.TrimSpace(options.DefaultGateway); value != "" {
		args = append(args, "-default-gateway", value)
	}
	if value := strings.TrimSpace(options.DefaultInterface); value != "" {
		args = append(args, "-default-interface", value)
	}
	if value := strings.TrimSpace(options.WSTunnelBinary); value != "" {
		args = append(args, "-wstunnel-binary", value)
	}
	if value := strings.TrimSpace(options.WireGuardInterface); value != "" {
		args = append(args, "-wireguard-interface", value)
	}
	if value := strings.TrimSpace(options.RuntimeRoot); value != "" {
		args = append(args, "-runtime-root", value)
	}
	return args
}

func isolatedOptionArgs(options planner.IsolatedAppOptions) []string {
	var args []string
	appendString := func(flag string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			args = append(args, flag, value)
		}
	}
	appendString("-session-id", options.SessionID)
	appendString("-app-id", options.AppID)
	if len(options.DNS) > 0 {
		args = append(args, "-dns", strings.Join(options.DNS, ","))
	}
	appendString("-wstunnel-binary", options.WSTunnelBinary)
	appendString("-wireguard-interface", options.WireGuardInterface)
	appendString("-runtime-root", options.RuntimeRoot)
	appendString("-namespace", options.Namespace)
	appendString("-host-veth", options.HostVeth)
	appendString("-namespace-veth", options.NamespaceVeth)
	appendString("-host-address", options.HostAddress)
	appendString("-namespace-address", options.NamespaceAddress)
	appendString("-host-gateway", options.HostGateway)
	appendString("-app-uid", options.LaunchUID)
	appendString("-app-gid", options.LaunchGID)
	for _, value := range options.LaunchEnv {
		appendString("-app-env", value)
	}
	return args
}

func profileOperationError(err error) OperationResponse {
	var validationErr *profile.ValidationError
	if errors.As(err, &validationErr) {
		return operationError(validationErr.Error())
	}
	return operationError(err.Error())
}

func operationError(message string) OperationResponse {
	return OperationResponse{OK: false, Output: message, Error: message}
}

func cliName() string {
	if stdruntime.GOOS == "windows" {
		return "big-red-button.exe"
	}
	return "big-red-button"
}
