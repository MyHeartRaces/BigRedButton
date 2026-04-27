package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	"github.com/MyHeartRaces/BigRedButton/internal/profile"
)

func TestRunConnectSuccess(t *testing.T) {
	plan := connectPlan(t)
	executor := &fakeExecutor{}

	result := New(executor).Run(context.Background(), plan)

	if result.State != StateConnected {
		t.Fatalf("state = %s error = %s rollback = %s", result.State, result.Error, result.RollbackError)
	}
	if len(result.AppliedStepIDs) != len(plan.Steps) {
		t.Fatalf("applied steps = %#v plan steps = %d", result.AppliedStepIDs, len(plan.Steps))
	}
	if len(result.RolledBackIDs) != 0 {
		t.Fatalf("unexpected rollback: %#v", result.RolledBackIDs)
	}
}

func TestRunConnectSkipsApplyTimeSteps(t *testing.T) {
	config := loadProfile(t)
	plan, err := planner.Connect(config, planner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{}

	result := New(executor).Run(context.Background(), plan)

	if result.State != StateConnected {
		t.Fatalf("state = %s error = %s", result.State, result.Error)
	}
	if !contains(result.SkippedStepIDs, "add-endpoint-route-exclusions") {
		t.Fatalf("expected apply-time route step to be skipped: %#v", result.SkippedStepIDs)
	}
	if contains(executor.applied, "add-endpoint-route-exclusions") {
		t.Fatalf("apply-time step should not be applied: %#v", executor.applied)
	}
}

func TestRunConnectFailureRollsBackAppliedStepsInReverseOrder(t *testing.T) {
	plan := connectPlan(t)
	executor := &fakeExecutor{failApply: "apply-wireguard-peer"}

	result := New(executor).Run(context.Background(), plan)

	if result.State != StateFailedRecoverable {
		t.Fatalf("state = %s error = %s rollback = %s", result.State, result.Error, result.RollbackError)
	}
	if result.FailedStepID != "apply-wireguard-peer" {
		t.Fatalf("failed step = %s", result.FailedStepID)
	}
	wantRollback := []string{
		"apply-wireguard-addresses",
		"create-wireguard-interface",
		"start-wstunnel",
		"add-route-exclusion-203-0-113-10",
	}
	if !reflect.DeepEqual(executor.rolledBack, wantRollback) {
		t.Fatalf("rollback order = %#v want %#v", executor.rolledBack, wantRollback)
	}
	if !reflect.DeepEqual(result.RolledBackIDs, wantRollback) {
		t.Fatalf("result rollback order = %#v want %#v", result.RolledBackIDs, wantRollback)
	}
}

func TestRunConnectRollbackFailureMarksDirty(t *testing.T) {
	plan := connectPlan(t)
	executor := &fakeExecutor{
		failApply:    "apply-wireguard-peer",
		failRollback: "start-wstunnel",
	}

	result := New(executor).Run(context.Background(), plan)

	if result.State != StateFailedDirty {
		t.Fatalf("state = %s error = %s rollback = %s", result.State, result.Error, result.RollbackError)
	}
	if result.RollbackError == "" {
		t.Fatal("expected rollback error")
	}
	if !contains(executor.rolledBack, "start-wstunnel") {
		t.Fatalf("expected attempted wstunnel rollback: %#v", executor.rolledBack)
	}
}

func TestRunDisconnectContinuesAfterStepFailure(t *testing.T) {
	plan, err := planner.Disconnect(planner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{failApply: "stop-wstunnel"}

	result := New(executor).Run(context.Background(), plan)

	if result.State != StateFailedDirty {
		t.Fatalf("state = %s error = %s", result.State, result.Error)
	}
	if result.FailedStepID != "stop-wstunnel" {
		t.Fatalf("failed step = %s", result.FailedStepID)
	}
	if !contains(executor.applied, "clear-runtime-state") {
		t.Fatalf("disconnect should continue cleanup after failure: %#v", executor.applied)
	}
	if len(executor.rolledBack) != 0 {
		t.Fatalf("disconnect should not rollback cleanup steps: %#v", executor.rolledBack)
	}
}

func TestRunRejectsUnsupportedPlanKind(t *testing.T) {
	result := New(&fakeExecutor{}).Run(context.Background(), planner.Plan{Kind: "unknown"})
	if result.State != StateFailedDirty {
		t.Fatalf("state = %s", result.State)
	}
	if result.Error == "" {
		t.Fatal("expected error")
	}
}

func connectPlan(t *testing.T) planner.Plan {
	t.Helper()
	plan, err := planner.Connect(loadProfile(t), planner.Options{EndpointIPs: []string{"203.0.113.10"}})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func loadProfile(t *testing.T) profile.Config {
	t.Helper()
	config, err := profile.LoadFile("../../testdata/profiles/valid-v7.json")
	if err != nil {
		t.Fatal(err)
	}
	return config
}

type fakeExecutor struct {
	failApply    string
	failRollback string
	applied      []string
	rolledBack   []string
}

func (f *fakeExecutor) Apply(_ context.Context, step planner.Step) error {
	if step.ID == f.failApply {
		return errors.New("apply failed")
	}
	f.applied = append(f.applied, step.ID)
	return nil
}

func (f *fakeExecutor) Rollback(_ context.Context, step planner.Step) error {
	f.rolledBack = append(f.rolledBack, step.ID)
	if step.ID == f.failRollback {
		return errors.New("rollback failed")
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
