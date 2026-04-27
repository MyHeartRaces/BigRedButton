package linux

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/tracegate/tracegate-launcher/internal/engine"
	"github.com/tracegate/tracegate-launcher/internal/planner"
	"github.com/tracegate/tracegate-launcher/internal/profile"
	truntime "github.com/tracegate/tracegate-launcher/internal/runtime"
)

func TestDryRunExecutorRecordsConcreteRouteCommands(t *testing.T) {
	plan := connectPlan(t, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
	})
	executor, err := NewDryRunExecutor(plan)
	if err != nil {
		t.Fatal(err)
	}

	result := engine.New(executor).Run(context.Background(), plan)
	if result.State != engine.StateConnected {
		t.Fatalf("state = %s error = %s", result.State, result.Error)
	}

	got := operationArgv(executor.Operations())
	want := [][]string{
		{"ip", "-4", "route", "get", "203.0.113.10"},
		{"ip", "-4", "route", "replace", "203.0.113.10/32", "via", "192.0.2.1", "dev", "eth0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v want %#v", got, want)
	}
	state, ok := executor.RuntimeState()
	if !ok {
		t.Fatal("expected runtime state")
	}
	if len(state.RouteExclusions) != 1 {
		t.Fatalf("runtime route exclusions = %#v", state.RouteExclusions)
	}
}

func TestDryRunExecutorFailsWhenRouteExclusionIsNotResolved(t *testing.T) {
	plan := connectPlan(t, planner.Options{
		EndpointIPs: []string{"203.0.113.10"},
	})
	executor, err := NewDryRunExecutor(plan)
	if err != nil {
		t.Fatal(err)
	}

	result := engine.New(executor).Run(context.Background(), plan)
	if result.State != engine.StateFailedRecoverable {
		t.Fatalf("state = %s error = %s", result.State, result.Error)
	}
	if result.FailedStepID != "add-route-exclusion-203-0-113-10" {
		t.Fatalf("failed step = %s", result.FailedStepID)
	}
	if !strings.Contains(result.Error, "route exclusion for endpoint 203.0.113.10 is not resolved") {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	got := operationArgv(executor.Operations())
	want := [][]string{{"ip", "-4", "route", "get", "203.0.113.10"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v want %#v", got, want)
	}
}

func TestDryRunExecutorUsesReadOnlyDiscovery(t *testing.T) {
	plan := connectPlan(t, planner.Options{
		EndpointIPs: []string{"203.0.113.10"},
	})
	runner := &fakeCommandRunner{
		output: "203.0.113.10 via 192.0.2.1 dev eth0 src 192.0.2.55 uid 1000 cache",
	}
	executor, err := NewDryRunExecutorWithOptions(plan, DryRunOptions{
		ReadOnlyDiscovery: true,
		Runner:            runner,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := engine.New(executor).Run(context.Background(), plan)
	if result.State != engine.StateConnected {
		t.Fatalf("state = %s error = %s", result.State, result.Error)
	}

	got := operationArgv(executor.Operations())
	want := [][]string{
		{"ip", "-4", "route", "get", "203.0.113.10"},
		{"ip", "-4", "route", "replace", "203.0.113.10/32", "via", "192.0.2.1", "dev", "eth0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v want %#v", got, want)
	}
	state, ok := executor.RuntimeState()
	if !ok {
		t.Fatal("expected runtime state")
	}
	if len(state.RouteExclusions) != 1 || state.RouteExclusions[0].Gateway != "192.0.2.1" {
		t.Fatalf("runtime state = %#v", state)
	}
}

func TestDryRunExecutorRecordsRouteRollbackCommand(t *testing.T) {
	plan := connectPlan(t, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
	})
	executor, err := NewDryRunExecutor(plan)
	if err != nil {
		t.Fatal(err)
	}

	step := findStep(plan, "add-route-exclusion-203-0-113-10")
	if err := executor.Rollback(context.Background(), step); err != nil {
		t.Fatal(err)
	}

	got := operationArgv(executor.Operations())
	want := [][]string{{"ip", "-4", "route", "delete", "203.0.113.10/32", "via", "192.0.2.1", "dev", "eth0"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v want %#v", got, want)
	}
}

func TestDryRunExecutorPersistsRuntimeStateAndDisconnectDeletesRoutes(t *testing.T) {
	runtimeRoot := t.TempDir()
	connect := connectPlan(t, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
		RuntimeRoot:      runtimeRoot,
	})
	connectExecutor, err := NewDryRunExecutorWithOptions(connect, DryRunOptions{
		PersistRuntime: true,
		RuntimeRoot:    runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	connectResult := engine.New(connectExecutor).Run(context.Background(), connect)
	if connectResult.State != engine.StateConnected {
		t.Fatalf("connect state = %s error = %s", connectResult.State, connectResult.Error)
	}

	disconnect, err := planner.Disconnect(planner.Options{RuntimeRoot: runtimeRoot})
	if err != nil {
		t.Fatal(err)
	}
	disconnectExecutor, err := NewDryRunExecutorWithOptions(disconnect, DryRunOptions{
		PersistRuntime: true,
		RuntimeRoot:    runtimeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	disconnectResult := engine.New(disconnectExecutor).Run(context.Background(), disconnect)
	if disconnectResult.State != engine.StateIdle {
		t.Fatalf("disconnect state = %s error = %s", disconnectResult.State, disconnectResult.Error)
	}

	got := operationArgv(disconnectExecutor.Operations())
	want := [][]string{{"ip", "-4", "route", "delete", "203.0.113.10/32", "via", "192.0.2.1", "dev", "eth0"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v want %#v", got, want)
	}
	store := truntime.Store{Root: runtimeRoot}
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("expected runtime state to be cleared")
	}
}

func connectPlan(t *testing.T, options planner.Options) planner.Plan {
	t.Helper()
	config, err := profile.LoadFile("../../../testdata/profiles/valid-v7.json")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planner.Connect(config, options)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func findStep(plan planner.Plan, id string) planner.Step {
	for _, step := range plan.Steps {
		if step.ID == id {
			return step
		}
	}
	return planner.Step{}
}

func operationArgv(operations []Operation) [][]string {
	argv := make([][]string, 0, len(operations))
	for _, operation := range operations {
		if operation.Command == nil {
			continue
		}
		argv = append(argv, operation.Command.Argv())
	}
	return argv
}
