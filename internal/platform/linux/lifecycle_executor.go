package linux

import (
	"context"
	"fmt"
	"strings"

	"github.com/tracegate/tracegate-launcher/internal/planner"
	"github.com/tracegate/tracegate-launcher/internal/profile"
	"github.com/tracegate/tracegate-launcher/internal/supervisor"
	"github.com/tracegate/tracegate-launcher/internal/wireguard"
)

type LifecycleExecutor struct {
	route     *RouteExecutor
	wstunnel  *supervisor.WSTunnelExecutor
	wireguard *WireGuardExecutor
}

type LifecycleExecutorOptions struct {
	Plan               planner.Plan
	Profile            profile.Config
	CommandRunner      CommandRunner
	ProcessRunner      supervisor.ProcessRunner
	WGConfigWriter     WireGuardConfigWriter
	WSTunnelBinary     string
	RuntimeRoot        string
	WireGuardIface     string
	WSTunnelLogLevel   string
	WSTunnelRemoteHost string
	WSTunnelRemotePort int
}

func NewLifecycleExecutor(options LifecycleExecutorOptions) (*LifecycleExecutor, error) {
	runtimeRoot := options.RuntimeRoot
	if runtimeRoot == "" {
		runtimeRoot = options.Plan.RuntimeRoot
	}
	wireGuardIface := options.WireGuardIface
	if wireGuardIface == "" {
		wireGuardIface = options.Plan.WireGuardInterface
	}

	routeExecutor, err := NewRouteExecutor(options.Plan, RouteExecutorOptions{
		Runner:      options.CommandRunner,
		RuntimeRoot: runtimeRoot,
	})
	if err != nil {
		return nil, err
	}

	wstunnelCommand, err := supervisor.WSTunnelClientCommand(supervisor.WSTunnelClientConfig{
		Binary:         firstNonEmpty(options.WSTunnelBinary, planner.DefaultWSTunnelBinary),
		ServerURL:      options.Profile.WSTunnelURL,
		PathPrefix:     options.Profile.WSTunnelPath,
		TLSServerName:  firstNonEmpty(options.Profile.WSTunnelTLSName, options.Profile.SNI),
		LocalUDPListen: options.Profile.LocalUDPListen,
		RemoteUDPHost:  options.WSTunnelRemoteHost,
		RemoteUDPPort:  options.WSTunnelRemotePort,
		LogLevel:       options.WSTunnelLogLevel,
	})
	if err != nil {
		return nil, err
	}
	wstunnelExecutor, err := supervisor.NewWSTunnelExecutor(supervisor.WSTunnelExecutorOptions{
		Command: wstunnelCommand,
		Runner:  options.ProcessRunner,
	})
	if err != nil {
		return nil, err
	}

	wireGuardExecutor, err := NewWireGuardExecutor(WireGuardExecutorOptions{
		Config:       wireguard.ConfigFromProfile(options.Profile, wireGuardIface),
		Runner:       options.CommandRunner,
		ConfigWriter: options.WGConfigWriter,
		RuntimeRoot:  runtimeRoot,
	})
	if err != nil {
		return nil, err
	}

	return &LifecycleExecutor{
		route:     routeExecutor,
		wstunnel:  wstunnelExecutor,
		wireguard: wireGuardExecutor,
	}, nil
}

func (e *LifecycleExecutor) Apply(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("linux lifecycle executor is nil")
	}
	if isNoopLifecycleStep(step) {
		return nil
	}
	if step.ID == "store-runtime-state" {
		return e.storeRuntimeState(ctx, step)
	}
	if isRouteExecutorStep("connect", step) || isRouteExecutorStep("disconnect", step) {
		return e.route.Apply(ctx, step)
	}
	if step.ID == "start-wstunnel" {
		return e.wstunnel.Apply(ctx, step)
	}
	if isWireGuardStep(step) {
		return e.wireguard.Apply(ctx, step)
	}
	return fmt.Errorf("unsupported linux lifecycle step: %s", step.ID)
}

func (e *LifecycleExecutor) Rollback(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("linux lifecycle executor is nil")
	}
	if isNoopLifecycleStep(step) {
		return nil
	}
	if isRouteRollbackStep(step) {
		return e.route.Rollback(ctx, step)
	}
	if step.ID == "start-wstunnel" {
		return e.wstunnel.Rollback(ctx, step)
	}
	if isWireGuardStep(step) {
		return e.wireguard.Rollback(ctx, step)
	}
	return nil
}

func (e *LifecycleExecutor) storeRuntimeState(ctx context.Context, step planner.Step) error {
	state, err := e.route.stateFromPlan()
	if err != nil {
		return err
	}
	if info, ok := e.wstunnel.ProcessInfo(); ok {
		state = state.WithWSTunnelProcess(info.PID, info.Command.Argv())
	}
	if err := e.route.store.Save(ctx, state); err != nil {
		return err
	}
	e.route.runtimeState = state
	e.route.recordRuntime(OperationApply, step.ID, "save "+e.route.storePath())
	return nil
}

func (e *LifecycleExecutor) RouteOperations() []Operation {
	if e == nil || e.route == nil {
		return nil
	}
	return e.route.Operations()
}

func (e *LifecycleExecutor) WSTunnelOperations() []supervisor.WSTunnelOperation {
	if e == nil || e.wstunnel == nil {
		return nil
	}
	return e.wstunnel.Operations()
}

func (e *LifecycleExecutor) WireGuardOperations() []Operation {
	if e == nil || e.wireguard == nil {
		return nil
	}
	return e.wireguard.Operations()
}

func isNoopLifecycleStep(step planner.Step) bool {
	switch step.ID {
	case "validate-profile", "resolve-wstunnel-host", "dns-plan", "verify-connected":
		return true
	default:
		return false
	}
}

func isRouteRollbackStep(step planner.Step) bool {
	return strings.HasPrefix(step.ID, "add-route-exclusion-")
}

func isWireGuardStep(step planner.Step) bool {
	switch step.ID {
	case "create-wireguard-interface",
		"apply-wireguard-addresses",
		"apply-wireguard-peer",
		"apply-client-routes",
		"remove-client-routes",
		"remove-wireguard-interface":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
