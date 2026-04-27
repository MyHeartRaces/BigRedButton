package linux

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tracegate/tracegate-launcher/internal/planner"
	"github.com/tracegate/tracegate-launcher/internal/routes"
	truntime "github.com/tracegate/tracegate-launcher/internal/runtime"
)

func TestRouteStepsSelectsOnlyRouteLifecycleSteps(t *testing.T) {
	connect := connectPlan(t, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
	})
	got := stepIDs(RouteSteps(connect))
	want := []string{
		"snapshot-route-203-0-113-10",
		"add-route-exclusion-203-0-113-10",
		"store-runtime-state",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("connect route steps = %#v want %#v", got, want)
	}

	disconnect, err := planner.Disconnect(planner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got = stepIDs(RouteSteps(disconnect))
	want = []string{
		"read-runtime-state",
		"remove-endpoint-route-exclusions",
		"clear-runtime-state",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("disconnect route steps = %#v want %#v", got, want)
	}
}

func TestRouteExecutorAppliesConnectRouteStepsAndPersistsState(t *testing.T) {
	runtimeRoot := t.TempDir()
	plan := connectPlan(t, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
		RuntimeRoot:      runtimeRoot,
	})
	runner := &recordingRunner{
		outputs: map[string]string{
			"ip -4 route get 203.0.113.10": "203.0.113.10 via 192.0.2.1 dev eth0 src 192.0.2.55 uid 1000 cache",
		},
	}
	executor, err := NewRouteExecutor(plan, RouteExecutorOptions{
		Runner:      runner,
		RuntimeRoot: runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, step := range RouteSteps(plan) {
		if err := executor.Apply(context.Background(), step); err != nil {
			t.Fatalf("apply %s: %v", step.ID, err)
		}
	}

	wantCommands := [][]string{
		{"ip", "-4", "route", "get", "203.0.113.10"},
		{"ip", "-4", "route", "replace", "203.0.113.10/32", "via", "192.0.2.1", "dev", "eth0"},
	}
	if !reflect.DeepEqual(runner.argv, wantCommands) {
		t.Fatalf("commands = %#v want %#v", runner.argv, wantCommands)
	}
	store := truntime.Store{Root: runtimeRoot}
	state, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.RouteExclusions) != 1 {
		t.Fatalf("route exclusions = %#v", state.RouteExclusions)
	}
}

func TestRouteExecutorUsesDiscoveredRouteWhenPlanHasNoGateway(t *testing.T) {
	runtimeRoot := t.TempDir()
	plan := connectPlan(t, planner.Options{
		EndpointIPs: []string{"203.0.113.10"},
		RuntimeRoot: runtimeRoot,
	})
	runner := &recordingRunner{
		outputs: map[string]string{
			"ip -4 route get 203.0.113.10": "203.0.113.10 via 192.0.2.1 dev eth0 src 192.0.2.55 uid 1000 cache",
		},
	}
	executor, err := NewRouteExecutor(plan, RouteExecutorOptions{
		Runner:      runner,
		RuntimeRoot: runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, step := range RouteSteps(plan) {
		if err := executor.Apply(context.Background(), step); err != nil {
			t.Fatalf("apply %s: %v", step.ID, err)
		}
	}

	wantCommands := [][]string{
		{"ip", "-4", "route", "get", "203.0.113.10"},
		{"ip", "-4", "route", "replace", "203.0.113.10/32", "via", "192.0.2.1", "dev", "eth0"},
	}
	if !reflect.DeepEqual(runner.argv, wantCommands) {
		t.Fatalf("commands = %#v want %#v", runner.argv, wantCommands)
	}
	state, ok := executor.RuntimeState()
	if !ok {
		t.Fatal("expected runtime state")
	}
	if len(state.RouteExclusions) != 1 || state.RouteExclusions[0].Gateway != "192.0.2.1" {
		t.Fatalf("state = %#v", state)
	}
}

func TestRouteExecutorDisconnectDeletesRoutesFromState(t *testing.T) {
	runtimeRoot := t.TempDir()
	exclusion, err := routes.NewEndpointExclusion("203.0.113.10", "192.0.2.1", "eth0")
	if err != nil {
		t.Fatal(err)
	}
	state := truntime.State{
		Version:            truntime.StateVersion,
		ProfileFingerprint: "abc123",
		WireGuardInterface: "tg-test",
		RouteExclusions:    []routes.EndpointExclusion{exclusion},
	}
	store := truntime.Store{Root: runtimeRoot}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	plan, err := planner.Disconnect(planner.Options{RuntimeRoot: runtimeRoot})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	executor, err := NewRouteExecutor(plan, RouteExecutorOptions{
		Runner:      runner,
		RuntimeRoot: runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, step := range RouteSteps(plan) {
		if err := executor.Apply(context.Background(), step); err != nil {
			t.Fatalf("apply %s: %v", step.ID, err)
		}
	}

	wantCommands := [][]string{{"ip", "-4", "route", "delete", "203.0.113.10/32", "via", "192.0.2.1", "dev", "eth0"}}
	if !reflect.DeepEqual(runner.argv, wantCommands) {
		t.Fatalf("commands = %#v want %#v", runner.argv, wantCommands)
	}
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("expected runtime state to be cleared")
	}
}

func TestRouteExecutorRollbackDeletesRoute(t *testing.T) {
	plan := connectPlan(t, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
	})
	runner := &recordingRunner{}
	executor, err := NewRouteExecutor(plan, RouteExecutorOptions{Runner: runner})
	if err != nil {
		t.Fatal(err)
	}

	if err := executor.Rollback(context.Background(), findStep(plan, "add-route-exclusion-203-0-113-10")); err != nil {
		t.Fatal(err)
	}

	wantCommands := [][]string{{"ip", "-4", "route", "delete", "203.0.113.10/32", "via", "192.0.2.1", "dev", "eth0"}}
	if !reflect.DeepEqual(runner.argv, wantCommands) {
		t.Fatalf("commands = %#v want %#v", runner.argv, wantCommands)
	}
}

func TestRouteExecutorReturnsCommandOutputOnFailure(t *testing.T) {
	plan := connectPlan(t, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
	})
	runner := &recordingRunner{
		errs: map[string]error{
			"ip -4 route get 203.0.113.10": errors.New("exit status 2"),
		},
		outputs: map[string]string{
			"ip -4 route get 203.0.113.10": "RTNETLINK answers: Network is unreachable",
		},
	}
	executor, err := NewRouteExecutor(plan, RouteExecutorOptions{Runner: runner})
	if err != nil {
		t.Fatal(err)
	}

	err = executor.Apply(context.Background(), findStep(plan, "snapshot-route-203-0-113-10"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Network is unreachable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRouteExecutorRejectsUnsupportedStep(t *testing.T) {
	plan := connectPlan(t, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
	})
	executor, err := NewRouteExecutor(plan, RouteExecutorOptions{Runner: &recordingRunner{}})
	if err != nil {
		t.Fatal(err)
	}

	err = executor.Apply(context.Background(), findStep(plan, "start-wstunnel"))
	if !errors.Is(err, ErrUnsupportedRouteStep) {
		t.Fatalf("expected unsupported step error, got %v", err)
	}
}

func stepIDs(steps []planner.Step) []string {
	ids := make([]string, 0, len(steps))
	for _, step := range steps {
		ids = append(ids, step.ID)
	}
	return ids
}

type recordingRunner struct {
	argv    [][]string
	outputs map[string]string
	errs    map[string]error
}

func (r *recordingRunner) Run(_ context.Context, command Command) ([]byte, error) {
	r.argv = append(r.argv, command.Argv())
	key := command.String()
	var output []byte
	if r.outputs != nil {
		output = []byte(r.outputs[key])
	}
	var err error
	if r.errs != nil {
		err = r.errs[key]
	}
	return output, err
}
