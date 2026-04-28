package linux

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MyHeartRaces/BigRedButton/internal/engine"
	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
	"github.com/MyHeartRaces/BigRedButton/internal/supervisor"
)

func TestIsolatedExecutorRunsSessionAndStop(t *testing.T) {
	runtimeRoot := t.TempDir()
	netNSConfigRoot := t.TempDir()
	sessionID := "123e4567-e89b-12d3-a456-426614174000"
	profileConfig := linuxProfile(t)
	plan, err := planner.IsolatedAppTunnel(profileConfig, planner.IsolatedAppOptions{
		SessionID:   sessionID,
		AppCommand:  []string{"/usr/bin/curl", "https://example.com"},
		RuntimeRoot: runtimeRoot,
		LaunchUID:   "1000",
		LaunchGID:   "1000",
		LaunchEnv:   []string{"DISPLAY=:1", "XDG_RUNTIME_DIR=/run/user/1000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	processRunner := &isolatedProcessRunner{nextPID: 101}
	executor, err := NewIsolatedExecutor(IsolatedExecutorOptions{
		Plan:            plan,
		Profile:         profileConfig,
		Runner:          runner,
		ProcessRunner:   processRunner,
		RuntimeRoot:     runtimeRoot,
		NetNSConfigRoot: netNSConfigRoot,
		LookPath:        foundLookPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := engine.New(executor).Run(context.Background(), plan)
	if result.State != engine.StateConnected {
		t.Fatalf("state = %s error = %s rollback = %s", result.State, result.Error, result.RollbackError)
	}
	if len(processRunner.started) != 2 {
		t.Fatalf("started processes = %#v", processRunner.started)
	}
	if processRunner.started[0].Name != "wstunnel" {
		t.Fatalf("unexpected wstunnel command: %#v", processRunner.started[0])
	}
	if processRunner.started[1].Name != "ip" || !strings.Contains(processRunner.started[1].String(), "netns exec brb-123e4567 setpriv --reuid 1000 --regid 1000 --init-groups env DISPLAY=:1 XDG_RUNTIME_DIR=/run/user/1000 /usr/bin/curl") {
		t.Fatalf("unexpected app command: %#v", processRunner.started[1])
	}

	sessionRoot := filepath.Join(runtimeRoot, planner.DefaultIsolatedRuntimeSubdir, sessionID)
	state, err := (truntime.Store{Root: sessionRoot}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Mode != planner.IsolatedAppTunnelKind || state.Namespace != "brb-123e4567" {
		t.Fatalf("unexpected isolated state: %#v", state)
	}
	if state.WSTunnelProcess == nil || state.WSTunnelProcess.PID != 101 {
		t.Fatalf("wstunnel state = %#v", state.WSTunnelProcess)
	}
	if state.AppProcess == nil || state.AppProcess.PID != 102 {
		t.Fatalf("app state = %#v", state.AppProcess)
	}
	if _, err := os.Stat(filepath.Join(sessionRoot, "wg-setconf.conf")); !os.IsNotExist(err) {
		t.Fatalf("expected temporary WireGuard config to be removed, err = %v", err)
	}
	dnsPath := filepath.Join(netNSConfigRoot, "brb-123e4567", "resolv.conf")
	dnsPayload, err := os.ReadFile(dnsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(dnsPayload) != "nameserver 1.1.1.1\n" {
		t.Fatalf("unexpected DNS payload: %q", dnsPayload)
	}
	ruleset, err := os.ReadFile(filepath.Join(sessionRoot, "namespace-killswitch.nft"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ruleset), "policy drop") || !strings.Contains(string(ruleset), `oifname "brbwg123e4567" accept`) {
		t.Fatalf("unexpected kill-switch ruleset:\n%s", ruleset)
	}
	commands := flattenArgv(runner.argv)
	for _, want := range []string{
		"ip netns add brb-123e4567",
		"ip link add brbh123e4567 type veth peer name brbn123e4567",
		"ip netns exec brb-123e4567 ip link add dev brbwg123e4567 type wireguard",
		"ip netns exec brb-123e4567 wg setconf brbwg123e4567 " + sessionRoot + "/wg-setconf.conf",
		"ip netns exec brb-123e4567 nft -f " + sessionRoot + "/namespace-killswitch.nft",
	} {
		if !strings.Contains(commands, want) {
			t.Fatalf("missing command %q in:\n%s", want, commands)
		}
	}

	stopPlan, err := planner.IsolatedAppStop(planner.IsolatedAppStopOptions{
		SessionID:   sessionID,
		RuntimeRoot: runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	stopRunner := &recordingRunner{}
	stopper := &lifecycleStopper{}
	stopExecutor, err := NewIsolatedExecutor(IsolatedExecutorOptions{
		Plan:            stopPlan,
		Runner:          stopRunner,
		ProcessStopper:  stopper,
		RuntimeRoot:     runtimeRoot,
		NetNSConfigRoot: netNSConfigRoot,
		LookPath:        foundLookPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	stopResult := engine.New(stopExecutor).Run(context.Background(), stopPlan)
	if stopResult.State != engine.StateIdle {
		t.Fatalf("stop state = %s error = %s", stopResult.State, stopResult.Error)
	}
	if len(stopper.stopped) != 2 || stopper.stopped[0] != 102 || stopper.stopped[1] != 101 {
		t.Fatalf("stopped pids = %#v", stopper.stopped)
	}
	stopCommands := flattenArgv(stopRunner.argv)
	for _, want := range []string{
		"ip netns pids brb-123e4567",
		"ip netns exec brb-123e4567 nft flush ruleset",
		"ip netns exec brb-123e4567 ip -4 route delete 0.0.0.0/0 dev brbwg123e4567",
		"ip netns exec brb-123e4567 ip link delete dev brbwg123e4567",
		"ip link delete dev brbh123e4567",
		"ip netns delete brb-123e4567",
	} {
		if !strings.Contains(stopCommands, want) {
			t.Fatalf("missing stop command %q in:\n%s", want, stopCommands)
		}
	}
	if _, err := os.Stat(dnsPath); !os.IsNotExist(err) {
		t.Fatalf("expected namespace DNS to be removed, err = %v", err)
	}
	if _, err := (truntime.Store{Root: sessionRoot}).Load(context.Background()); err == nil {
		t.Fatal("expected isolated runtime state to be cleared")
	}
}

func TestIsolatedExecutorFailsBeforeMutationWhenPrerequisiteMissing(t *testing.T) {
	runtimeRoot := t.TempDir()
	sessionID := "123e4567-e89b-12d3-a456-426614174000"
	profileConfig := linuxProfile(t)
	plan, err := planner.IsolatedAppTunnel(profileConfig, planner.IsolatedAppOptions{
		SessionID:   sessionID,
		AppCommand:  []string{"/usr/bin/curl", "https://example.com"},
		RuntimeRoot: runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	executor, err := NewIsolatedExecutor(IsolatedExecutorOptions{
		Plan:        plan,
		Profile:     profileConfig,
		Runner:      runner,
		RuntimeRoot: runtimeRoot,
		LookPath: func(binary string) (string, error) {
			if binary == "nft" {
				return "", os.ErrNotExist
			}
			return "/usr/bin/" + binary, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := engine.New(executor).Run(context.Background(), plan)
	if result.State != engine.StateFailedRecoverable {
		t.Fatalf("state = %s error = %s", result.State, result.Error)
	}
	if result.FailedStepID != "validate-linux-prerequisites" {
		t.Fatalf("failed step = %s", result.FailedStepID)
	}
	if !strings.Contains(result.Error, "required binary nft was not found") {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if len(runner.argv) != 0 {
		t.Fatalf("expected no Linux mutations, got %#v", runner.argv)
	}
	if _, err := os.Stat(filepath.Join(runtimeRoot, planner.DefaultIsolatedRuntimeSubdir, sessionID)); !os.IsNotExist(err) {
		t.Fatalf("expected no runtime root, err = %v", err)
	}
}

func TestIsolatedExecutorRefusesExistingSessionState(t *testing.T) {
	runtimeRoot := t.TempDir()
	sessionID := "123e4567-e89b-12d3-a456-426614174000"
	sessionRoot := filepath.Join(runtimeRoot, planner.DefaultIsolatedRuntimeSubdir, sessionID)
	err := (truntime.Store{Root: sessionRoot}).Save(context.Background(), truntime.State{
		Version:            truntime.StateVersion,
		Mode:               planner.IsolatedAppTunnelKind,
		ProfileFingerprint: "abc123",
		WireGuardInterface: "brbwg123e4567",
		SessionID:          sessionID,
		Namespace:          "brb-123e4567",
		HostVeth:           "brbh123e4567",
		NamespaceVeth:      "brbn123e4567",
	})
	if err != nil {
		t.Fatal(err)
	}
	profileConfig := linuxProfile(t)
	plan, err := planner.IsolatedAppTunnel(profileConfig, planner.IsolatedAppOptions{
		SessionID:   sessionID,
		AppCommand:  []string{"/usr/bin/curl", "https://example.com"},
		RuntimeRoot: runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	processRunner := &isolatedProcessRunner{nextPID: 101}
	executor, err := NewIsolatedExecutor(IsolatedExecutorOptions{
		Plan:            plan,
		Profile:         profileConfig,
		Runner:          runner,
		ProcessRunner:   processRunner,
		RuntimeRoot:     runtimeRoot,
		NetNSConfigRoot: t.TempDir(),
		LookPath:        foundLookPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := engine.New(executor).Run(context.Background(), plan)
	if result.State != engine.StateFailedRecoverable {
		t.Fatalf("state = %s error = %s", result.State, result.Error)
	}
	if result.FailedStepID != "create-isolated-runtime-root" {
		t.Fatalf("failed step = %s", result.FailedStepID)
	}
	if !strings.Contains(result.Error, "runtime state already exists") {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if len(runner.argv) != 0 {
		t.Fatalf("expected no Linux mutations, got %#v", runner.argv)
	}
	if len(processRunner.started) != 0 {
		t.Fatalf("expected no started processes, got %#v", processRunner.started)
	}
}

func TestIsolatedExecutorCleanupIsBestEffort(t *testing.T) {
	runtimeRoot := t.TempDir()
	netNSConfigRoot := t.TempDir()
	sessionID := "123e4567-e89b-12d3-a456-426614174000"
	sessionRoot := filepath.Join(runtimeRoot, planner.DefaultIsolatedRuntimeSubdir, sessionID)
	if err := os.MkdirAll(sessionRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	dnsRoot := filepath.Join(netNSConfigRoot, "brb-123e4567")
	if err := os.MkdirAll(dnsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dnsRoot, "resolv.conf"), []byte("nameserver 1.1.1.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dnsRoot, "leftover.conf"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := planner.IsolatedAppCleanup(planner.IsolatedAppStopOptions{
		SessionID:   sessionID,
		RuntimeRoot: runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{
		outputs: map[string]string{
			"ip netns pids brb-123e4567":      "201\nbad\n202\n",
			"ip link delete dev brbh123e4567": "already gone",
		},
		errs: map[string]error{
			"ip netns exec brb-123e4567 nft flush ruleset":                errors.New("missing namespace"),
			"ip netns exec brb-123e4567 ip link delete dev brbwg123e4567": errors.New("missing wireguard"),
			"ip link delete dev brbh123e4567":                             errors.New("missing host veth"),
			"ip netns delete brb-123e4567":                                errors.New("missing namespace"),
		},
	}
	stopper := &lifecycleStopper{}
	executor, err := NewIsolatedExecutor(IsolatedExecutorOptions{
		Plan:            plan,
		Runner:          runner,
		ProcessStopper:  stopper,
		RuntimeRoot:     runtimeRoot,
		NetNSConfigRoot: netNSConfigRoot,
		LookPath:        foundLookPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := engine.New(executor).Run(context.Background(), plan)
	if result.State != engine.StateIdle {
		t.Fatalf("cleanup state = %s error = %s", result.State, result.Error)
	}
	if len(stopper.stopped) != 2 || stopper.stopped[0] != 201 || stopper.stopped[1] != 202 {
		t.Fatalf("stopped pids = %#v", stopper.stopped)
	}
	if _, err := os.Stat(sessionRoot); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove runtime root, err = %v", err)
	}
	if _, err := os.Stat(dnsRoot); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove namespace DNS root, err = %v", err)
	}
	commands := flattenArgv(runner.argv)
	for _, want := range []string{
		"ip netns pids brb-123e4567",
		"ip netns exec brb-123e4567 nft flush ruleset",
		"ip netns exec brb-123e4567 ip link delete dev brbwg123e4567",
		"ip link delete dev brbh123e4567",
		"ip netns delete brb-123e4567",
	} {
		if !strings.Contains(commands, want) {
			t.Fatalf("missing cleanup command %q in:\n%s", want, commands)
		}
	}
	operations := executor.Operations()
	if !strings.Contains(operationStrings(operations), "ignore cleanup error") {
		t.Fatalf("expected ignored cleanup errors in operations: %#v", operations)
	}
}

func foundLookPath(binary string) (string, error) {
	return "/usr/bin/" + binary, nil
}

type isolatedProcessRunner struct {
	nextPID int
	started []supervisor.Command
}

func (r *isolatedProcessRunner) Start(_ context.Context, command supervisor.Command) (supervisor.Process, error) {
	r.started = append(r.started, command)
	pid := r.nextPID
	if pid == 0 {
		pid = 1
	}
	r.nextPID = pid + 1
	return &lifecycleProcess{info: supervisor.ProcessInfo{PID: pid, Command: command}}, nil
}
