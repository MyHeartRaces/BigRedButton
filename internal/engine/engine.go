package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
)

type State string

const (
	StateIdle              State = "Idle"
	StateConnecting        State = "Connecting"
	StateConnected         State = "Connected"
	StateDisconnecting     State = "Disconnecting"
	StateFailedRecoverable State = "FailedRecoverable"
	StateFailedDirty       State = "FailedDirty"
)

type Phase string

const (
	PhaseApply    Phase = "apply"
	PhaseRollback Phase = "rollback"
	PhaseSkip     Phase = "skip"
)

type Status string

const (
	StatusOK      Status = "ok"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

type Executor interface {
	Apply(ctx context.Context, step planner.Step) error
	Rollback(ctx context.Context, step planner.Step) error
}

type Engine struct {
	executor Executor
}

type Result struct {
	PlanKind       string   `json:"plan_kind"`
	State          State    `json:"state"`
	AppliedStepIDs []string `json:"applied_step_ids,omitempty"`
	RolledBackIDs  []string `json:"rolled_back_ids,omitempty"`
	SkippedStepIDs []string `json:"skipped_step_ids,omitempty"`
	FailedStepID   string   `json:"failed_step_id,omitempty"`
	Error          string   `json:"error,omitempty"`
	RollbackError  string   `json:"rollback_error,omitempty"`
	Events         []Event  `json:"events,omitempty"`
}

type Event struct {
	Phase  Phase  `json:"phase"`
	Status Status `json:"status"`
	StepID string `json:"step_id"`
	Error  string `json:"error,omitempty"`
}

func New(executor Executor) *Engine {
	return &Engine{executor: executor}
}

func (e *Engine) Run(ctx context.Context, plan planner.Plan) Result {
	if e == nil || e.executor == nil {
		return Result{
			PlanKind: plan.Kind,
			State:    StateFailedDirty,
			Error:    "engine executor is nil",
		}
	}

	switch plan.Kind {
	case "connect", planner.IsolatedAppTunnelKind:
		return e.runConnect(ctx, plan)
	case "disconnect", planner.IsolatedAppStopKind, planner.IsolatedAppCleanupKind:
		return e.runDisconnect(ctx, plan)
	default:
		return Result{
			PlanKind: plan.Kind,
			State:    StateFailedDirty,
			Error:    fmt.Sprintf("unsupported plan kind: %s", plan.Kind),
		}
	}
}

func (e *Engine) runConnect(ctx context.Context, plan planner.Plan) Result {
	result := Result{PlanKind: plan.Kind, State: StateConnecting}
	var applied []planner.Step

	for _, step := range plan.Steps {
		if step.SkippedUntilApply {
			result.SkippedStepIDs = append(result.SkippedStepIDs, step.ID)
			result.Events = append(result.Events, Event{Phase: PhaseSkip, Status: StatusSkipped, StepID: step.ID})
			continue
		}
		if err := e.executor.Apply(ctx, step); err != nil {
			result.FailedStepID = step.ID
			result.Error = err.Error()
			result.Events = append(result.Events, Event{Phase: PhaseApply, Status: StatusFailed, StepID: step.ID, Error: err.Error()})
			result.State = StateFailedRecoverable
			if rollbackErr := e.rollbackApplied(ctx, applied, &result); rollbackErr != nil {
				result.State = StateFailedDirty
				result.RollbackError = rollbackErr.Error()
			}
			return result
		}
		result.AppliedStepIDs = append(result.AppliedStepIDs, step.ID)
		result.Events = append(result.Events, Event{Phase: PhaseApply, Status: StatusOK, StepID: step.ID})
		applied = append(applied, step)
	}

	result.State = StateConnected
	return result
}

func (e *Engine) runDisconnect(ctx context.Context, plan planner.Plan) Result {
	result := Result{PlanKind: plan.Kind, State: StateDisconnecting}
	var stepErrors []error

	for _, step := range plan.Steps {
		if step.SkippedUntilApply {
			result.SkippedStepIDs = append(result.SkippedStepIDs, step.ID)
			result.Events = append(result.Events, Event{Phase: PhaseSkip, Status: StatusSkipped, StepID: step.ID})
			continue
		}
		if err := e.executor.Apply(ctx, step); err != nil {
			if result.FailedStepID == "" {
				result.FailedStepID = step.ID
			}
			stepErrors = append(stepErrors, fmt.Errorf("%s: %w", step.ID, err))
			result.Events = append(result.Events, Event{Phase: PhaseApply, Status: StatusFailed, StepID: step.ID, Error: err.Error()})
			continue
		}
		result.AppliedStepIDs = append(result.AppliedStepIDs, step.ID)
		result.Events = append(result.Events, Event{Phase: PhaseApply, Status: StatusOK, StepID: step.ID})
	}

	if len(stepErrors) > 0 {
		result.State = StateFailedDirty
		result.Error = errors.Join(stepErrors...).Error()
		return result
	}
	result.State = StateIdle
	return result
}

func (e *Engine) rollbackApplied(ctx context.Context, applied []planner.Step, result *Result) error {
	var rollbackErrors []error
	for index := len(applied) - 1; index >= 0; index-- {
		step := applied[index]
		if len(step.Rollback) == 0 {
			continue
		}
		if err := e.executor.Rollback(ctx, step); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("%s: %w", step.ID, err))
			result.Events = append(result.Events, Event{Phase: PhaseRollback, Status: StatusFailed, StepID: step.ID, Error: err.Error()})
			continue
		}
		result.RolledBackIDs = append(result.RolledBackIDs, step.ID)
		result.Events = append(result.Events, Event{Phase: PhaseRollback, Status: StatusOK, StepID: step.ID})
	}
	return errors.Join(rollbackErrors...)
}

type NoopExecutor struct{}

func (NoopExecutor) Apply(context.Context, planner.Step) error {
	return nil
}

func (NoopExecutor) Rollback(context.Context, planner.Step) error {
	return nil
}
