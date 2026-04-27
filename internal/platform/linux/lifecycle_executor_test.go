package linux

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/MyHeartRaces/BigRedButton/internal/engine"
	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	"github.com/MyHeartRaces/BigRedButton/internal/profile"
	"github.com/MyHeartRaces/BigRedButton/internal/routes"
	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
	"github.com/MyHeartRaces/BigRedButton/internal/supervisor"
)

func TestLifecycleExecutorRunsConnectPlan(t *testing.T) {
	runtimeRoot := t.TempDir()
	profileConfig := linuxProfile(t)
	plan, err := planner.Connect(profileConfig, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
		RuntimeRoot:      runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{
		outputs: map[string]string{
			"ip -4 route get 203.0.113.10": "203.0.113.10 via 192.0.2.1 dev eth0 src 192.0.2.55 uid 1000 cache",
		},
	}
	processRunner := &lifecycleProcessRunner{}
	writer := &memoryConfigWriter{path: runtimeRoot + "/wg-setconf.conf"}
	executor, err := NewLifecycleExecutor(LifecycleExecutorOptions{
		Plan:           plan,
		Profile:        profileConfig,
		CommandRunner:  runner,
		ProcessRunner:  processRunner,
		WGConfigWriter: writer,
		RuntimeRoot:    runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := engine.New(executor).Run(context.Background(), plan)
	if result.State != engine.StateConnected {
		t.Fatalf("state = %s error = %s rollback = %s", result.State, result.Error, result.RollbackError)
	}
	if len(processRunner.started) != 1 {
		t.Fatalf("started processes = %#v", processRunner.started)
	}
	if processRunner.started[0].Args[0] != "client" {
		t.Fatalf("unexpected wstunnel command: %#v", processRunner.started[0])
	}
	if !writer.cleaned {
		t.Fatal("expected wireguard secret config cleanup")
	}
	state, err := (truntime.Store{Root: runtimeRoot}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.WSTunnelProcess == nil || state.WSTunnelProcess.PID != 1 {
		t.Fatalf("runtime state process = %#v", state.WSTunnelProcess)
	}
	if len(state.WireGuardAllowedIPs) != 2 {
		t.Fatalf("runtime state allowed IPs = %#v", state.WireGuardAllowedIPs)
	}
	commands := flattenArgv(runner.argv)
	for _, want := range []string{
		"ip -4 route replace 203.0.113.10/32 via 192.0.2.1 dev eth0",
		"ip link add dev tg-v7 type wireguard",
		"wg setconf tg-v7 " + runtimeRoot + "/wg-setconf.conf",
		"ip -4 route replace 0.0.0.0/0 dev tg-v7",
		"ip -6 route replace ::/0 dev tg-v7",
	} {
		if !strings.Contains(commands, want) {
			t.Fatalf("missing %q in commands:\n%s", want, commands)
		}
	}
}

func TestLifecycleExecutorRollsBackOnWireGuardFailure(t *testing.T) {
	runtimeRoot := t.TempDir()
	profileConfig := linuxProfile(t)
	plan, err := planner.Connect(profileConfig, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
		RuntimeRoot:      runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	wgSetConf := "wg setconf tg-v7 " + runtimeRoot + "/wg-setconf.conf"
	runner := &recordingRunner{
		outputs: map[string]string{
			"ip -4 route get 203.0.113.10": "203.0.113.10 via 192.0.2.1 dev eth0 src 192.0.2.55 uid 1000 cache",
			wgSetConf:                      "wg setconf failed",
		},
		errs: map[string]error{
			wgSetConf: errors.New("exit status 1"),
		},
	}
	process := &lifecycleProcess{info: supervisor.ProcessInfo{PID: 777}}
	processRunner := &lifecycleProcessRunner{process: process}
	writer := &memoryConfigWriter{path: runtimeRoot + "/wg-setconf.conf"}
	executor, err := NewLifecycleExecutor(LifecycleExecutorOptions{
		Plan:           plan,
		Profile:        profileConfig,
		CommandRunner:  runner,
		ProcessRunner:  processRunner,
		WGConfigWriter: writer,
		RuntimeRoot:    runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := engine.New(executor).Run(context.Background(), plan)
	if result.State != engine.StateFailedRecoverable {
		t.Fatalf("state = %s error = %s rollback = %s", result.State, result.Error, result.RollbackError)
	}
	if result.FailedStepID != "apply-wireguard-peer" {
		t.Fatalf("failed step = %s", result.FailedStepID)
	}
	if !process.stopped {
		t.Fatal("expected wstunnel process rollback")
	}
	commands := flattenArgv(runner.argv)
	for _, want := range []string{
		"ip link delete dev tg-v7",
		"ip -4 route delete 203.0.113.10/32 via 192.0.2.1 dev eth0",
	} {
		if !strings.Contains(commands, want) {
			t.Fatalf("missing rollback command %q in:\n%s", want, commands)
		}
	}
}

func TestLifecycleExecutorRunsDisconnectPlanFromRuntimeState(t *testing.T) {
	runtimeRoot := t.TempDir()
	profileConfig := linuxProfile(t)
	exclusion, err := routes.NewEndpointExclusion("203.0.113.10", "192.0.2.1", "eth0")
	if err != nil {
		t.Fatal(err)
	}
	state := truntime.State{
		Version:             truntime.StateVersion,
		ProfileFingerprint:  "abc123",
		WireGuardInterface:  "tg-v7",
		RouteExclusions:     []routes.EndpointExclusion{exclusion},
		WireGuardAllowedIPs: []string{"0.0.0.0/0", "::/0"},
	}.WithWSTunnelProcess(4242, []string{"wstunnel", "client"})
	if err := (truntime.Store{Root: runtimeRoot}).Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	plan, err := planner.Disconnect(planner.Options{RuntimeRoot: runtimeRoot})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	stopper := &lifecycleStopper{}
	executor, err := NewLifecycleExecutor(LifecycleExecutorOptions{
		Plan:           plan,
		Profile:        profileConfig,
		CommandRunner:  runner,
		ProcessStopper: stopper,
		RuntimeRoot:    runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := engine.New(executor).Run(context.Background(), plan)
	if result.State != engine.StateIdle {
		t.Fatalf("state = %s error = %s", result.State, result.Error)
	}
	if len(stopper.stopped) != 1 || stopper.stopped[0] != 4242 {
		t.Fatalf("stopped pids = %#v", stopper.stopped)
	}
	commands := flattenArgv(runner.argv)
	for _, want := range []string{
		"ip -4 route delete 0.0.0.0/0 dev tg-v7",
		"ip -6 route delete ::/0 dev tg-v7",
		"ip link delete dev tg-v7",
		"ip -4 route delete 203.0.113.10/32 via 192.0.2.1 dev eth0",
	} {
		if !strings.Contains(commands, want) {
			t.Fatalf("missing disconnect command %q in:\n%s", want, commands)
		}
	}
	if _, err := (truntime.Store{Root: runtimeRoot}).Load(context.Background()); err == nil {
		t.Fatal("expected runtime state to be cleared")
	}
}

func linuxProfile(t *testing.T) profile.Config {
	t.Helper()
	config, err := profile.LoadFile("../../../testdata/profiles/valid-v7.json")
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func flattenArgv(values [][]string) string {
	var builder strings.Builder
	for _, argv := range values {
		builder.WriteString(strings.Join(argv, " "))
		builder.WriteByte('\n')
	}
	return builder.String()
}

type lifecycleProcessRunner struct {
	process *lifecycleProcess
	started []supervisor.Command
}

func (r *lifecycleProcessRunner) Start(_ context.Context, command supervisor.Command) (supervisor.Process, error) {
	r.started = append(r.started, command)
	if r.process == nil {
		r.process = &lifecycleProcess{info: supervisor.ProcessInfo{PID: 1, Command: command}}
	}
	r.process.info.Command = command
	return r.process, nil
}

type lifecycleProcess struct {
	info    supervisor.ProcessInfo
	stopped bool
}

func (p *lifecycleProcess) Info() supervisor.ProcessInfo {
	return p.info
}

func (p *lifecycleProcess) Stop(context.Context) error {
	p.stopped = true
	return nil
}

type lifecycleStopper struct {
	stopped []int
	err     error
}

func (s *lifecycleStopper) StopPID(_ context.Context, pid int) error {
	s.stopped = append(s.stopped, pid)
	return s.err
}
