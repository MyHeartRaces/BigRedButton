package linux

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	"github.com/MyHeartRaces/BigRedButton/internal/profile"
	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
	"github.com/MyHeartRaces/BigRedButton/internal/supervisor"
	"github.com/MyHeartRaces/BigRedButton/internal/wireguard"
)

const DefaultNetNSConfigRoot = "/etc/netns"

type IsolatedExecutor struct {
	plan               planner.Plan
	profile            profile.Config
	runner             CommandRunner
	processRunner      supervisor.ProcessRunner
	stopper            supervisor.ProcessStopper
	store              truntime.Store
	sessionRuntimeRoot string
	netNSConfigRoot    string
	lookPath           func(string) (string, error)
	runtimeState       truntime.State
	wstunnelProcess    supervisor.Process
	appProcess         supervisor.Process
	operations         []Operation
}

type IsolatedExecutorOptions struct {
	Plan            planner.Plan
	Profile         profile.Config
	Runner          CommandRunner
	ProcessRunner   supervisor.ProcessRunner
	ProcessStopper  supervisor.ProcessStopper
	RuntimeRoot     string
	NetNSConfigRoot string
	LookPath        func(string) (string, error)
}

type commandBuild struct {
	command Command
	err     error
}

func NewIsolatedExecutor(options IsolatedExecutorOptions) (*IsolatedExecutor, error) {
	if options.Plan.Kind != planner.IsolatedAppTunnelKind && options.Plan.Kind != planner.IsolatedAppStopKind && options.Plan.Kind != planner.IsolatedAppCleanupKind {
		return nil, fmt.Errorf("unsupported isolated executor plan kind: %s", options.Plan.Kind)
	}
	runtimeRoot := strings.TrimSpace(options.RuntimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = options.Plan.RuntimeRoot
	}
	if runtimeRoot == "" {
		runtimeRoot = planner.DefaultRuntimeRoot
	}
	sessionID := strings.TrimSpace(options.Plan.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("isolated executor session ID is required")
	}
	sessionRuntimeRoot := filepath.Join(runtimeRoot, planner.DefaultIsolatedRuntimeSubdir, sessionID)
	runner := options.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	processRunner := options.ProcessRunner
	if processRunner == nil {
		processRunner = supervisor.ExecProcessRunner{}
	}
	stopper := options.ProcessStopper
	if stopper == nil {
		stopper = supervisor.OSProcessStopper{}
	}
	netNSConfigRoot := strings.TrimSpace(options.NetNSConfigRoot)
	if netNSConfigRoot == "" {
		netNSConfigRoot = DefaultNetNSConfigRoot
	}
	lookPath := defaultLookPath(options.LookPath)
	return &IsolatedExecutor{
		plan:               options.Plan,
		profile:            options.Profile,
		runner:             runner,
		processRunner:      processRunner,
		stopper:            stopper,
		store:              truntime.Store{Root: sessionRuntimeRoot},
		sessionRuntimeRoot: sessionRuntimeRoot,
		netNSConfigRoot:    netNSConfigRoot,
		lookPath:           lookPath,
	}, nil
}

func (e *IsolatedExecutor) Apply(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("linux isolated executor is nil")
	}

	switch step.ID {
	case "validate-profile", "validate-app-command":
		return nil
	case "validate-linux-prerequisites":
		return e.validatePrerequisites(step)
	case "create-isolated-runtime-root":
		return e.createRuntimeRoot(ctx, step)
	case "create-netns":
		return e.runBuilt(ctx, OperationApply, step.ID, builtCommand(NetNSAddCommand(requiredDetailValue(step, "namespace"))))
	case "create-veth-pair":
		return e.createVethPair(ctx, step)
	case "configure-host-veth":
		return e.configureHostVeth(ctx, step)
	case "configure-namespace-veth":
		return e.configureNamespaceVeth(ctx, step)
	case "configure-namespace-dns":
		return e.configureNamespaceDNS(ctx, step)
	case "start-wstunnel-control":
		return e.startWSTunnelControl(ctx, step)
	case "create-wireguard-interface-in-netns":
		return e.runNamespacedBuilt(ctx, OperationApply, step.ID, requiredDetailValue(step, "namespace"), builtCommand(WireGuardCreateInterfaceCommand(requiredDetailValue(step, "interface"))))
	case "apply-wireguard-addresses-in-netns":
		return e.applyWireGuardAddresses(ctx, step)
	case "apply-wireguard-peer-in-netns":
		return e.applyWireGuardPeer(ctx, step)
	case "apply-namespace-client-routes":
		return e.applyNamespaceClientRoutes(ctx, step)
	case "apply-namespace-kill-switch":
		return e.applyNamespaceKillSwitch(ctx, step)
	case "launch-app-in-netns":
		return e.launchApp(ctx, step)
	case "monitor-process-tree":
		e.recordRuntime(OperationApply, step.ID, "monitor process tree for session "+requiredDetailValue(step, "session_id"))
		return nil
	case "store-isolated-runtime-state":
		return e.storeRuntimeState(ctx, step)
	case "read-isolated-runtime-state":
		return e.readRuntimeState(ctx, step)
	case "stop-isolated-app":
		return e.stopIsolatedApp(ctx, step)
	case "remove-namespace-kill-switch":
		state, err := e.stateForStop(ctx)
		if err != nil {
			return err
		}
		return e.runNamespaced(ctx, OperationApply, step.ID, state.Namespace, NftFlushRulesetCommand())
	case "remove-namespace-client-routes":
		return e.removeNamespaceClientRoutes(ctx, step)
	case "remove-wireguard-interface-in-netns":
		state, err := e.stateForStop(ctx)
		if err != nil {
			return err
		}
		return e.runNamespacedBuilt(ctx, OperationApply, step.ID, state.Namespace, builtCommand(WireGuardDeleteInterfaceCommand(state.WireGuardInterface)))
	case "stop-wstunnel-control":
		return e.stopRuntimeProcess(ctx, step.ID, "wstunnel", e.runtimeState.WSTunnelProcess)
	case "remove-namespace-dns":
		return e.removeNamespaceDNS(ctx, step)
	case "delete-netns":
		state, err := e.stateForStop(ctx)
		if err != nil {
			return err
		}
		if state.HostVeth != "" {
			if err := e.runBuilt(ctx, OperationApply, step.ID, builtCommand(LinkDeleteCommand(state.HostVeth))); err != nil {
				return err
			}
		}
		return e.runBuilt(ctx, OperationApply, step.ID, builtCommand(NetNSDeleteCommand(state.Namespace)))
	case "clear-isolated-runtime-state":
		return e.clearRuntimeState(ctx, step)
	case "cleanup-isolated-processes":
		return e.cleanupIsolatedProcesses(ctx, step)
	case "cleanup-namespace-kill-switch":
		return e.runNamespacedBestEffort(ctx, OperationApply, step.ID, requiredDetailValue(step, "namespace"), NftFlushRulesetCommand())
	case "cleanup-wireguard-interface-in-netns":
		return e.runNamespacedBestEffortBuilt(ctx, OperationApply, step.ID, requiredDetailValue(step, "namespace"), builtCommand(WireGuardDeleteInterfaceCommand(requiredDetailValue(step, "interface"))))
	case "cleanup-netns":
		hostVeth := requiredDetailValue(step, "host_veth")
		if hostVeth != "" {
			if err := e.runBestEffortBuilt(ctx, OperationApply, step.ID, builtCommand(LinkDeleteCommand(hostVeth))); err != nil {
				return err
			}
		}
		return e.runBestEffortBuilt(ctx, OperationApply, step.ID, builtCommand(NetNSDeleteCommand(requiredDetailValue(step, "namespace"))))
	case "cleanup-namespace-dns":
		return e.cleanupNamespaceDNS(ctx, step)
	case "cleanup-isolated-runtime-root":
		return e.removeRuntimeRoot(ctx, step)
	default:
		return fmt.Errorf("unsupported linux isolated step: %s", step.ID)
	}
}

func (e *IsolatedExecutor) Rollback(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("linux isolated executor is nil")
	}

	switch step.ID {
	case "create-isolated-runtime-root":
		e.recordRuntime(OperationRollback, step.ID, "remove "+e.sessionRuntimeRoot)
		return os.RemoveAll(e.sessionRuntimeRoot)
	case "create-netns":
		return e.runBuilt(ctx, OperationRollback, step.ID, builtCommand(NetNSDeleteCommand(requiredDetailValue(step, "namespace"))))
	case "create-veth-pair", "configure-host-veth":
		return e.runBuilt(ctx, OperationRollback, step.ID, builtCommand(LinkDeleteCommand(requiredDetailValue(step, "host_veth"))))
	case "configure-namespace-dns":
		return e.removeNamespaceDNS(ctx, step)
	case "start-wstunnel-control":
		if e.wstunnelProcess == nil {
			e.recordRuntime(OperationRollback, step.ID, "no WSTunnel process started")
			return nil
		}
		err := e.wstunnelProcess.Stop(ctx)
		e.recordRuntime(OperationRollback, step.ID, "stop WSTunnel control process")
		return err
	case "create-wireguard-interface-in-netns", "apply-wireguard-addresses-in-netns", "apply-wireguard-peer-in-netns":
		return e.runNamespacedBuilt(ctx, OperationRollback, step.ID, requiredDetailValue(step, "namespace"), builtCommand(WireGuardDeleteInterfaceCommand(requiredDetailValue(step, "interface"))))
	case "apply-namespace-client-routes":
		return e.deleteNamespaceClientRoutes(ctx, OperationRollback, step.ID, requiredDetailValue(step, "namespace"), requiredDetailValue(step, "interface"), detailValues(step, "allowed_ip"))
	case "apply-namespace-kill-switch":
		return e.runNamespaced(ctx, OperationRollback, step.ID, requiredDetailValue(step, "namespace"), NftFlushRulesetCommand())
	case "launch-app-in-netns":
		if e.appProcess == nil {
			e.recordRuntime(OperationRollback, step.ID, "no isolated app process started")
			return nil
		}
		err := e.appProcess.Stop(ctx)
		e.recordRuntime(OperationRollback, step.ID, "stop isolated app process")
		return err
	case "store-isolated-runtime-state":
		return e.clearRuntimeState(ctx, step)
	default:
		return nil
	}
}

func (e *IsolatedExecutor) Operations() []Operation {
	if e == nil {
		return nil
	}
	operations := make([]Operation, len(e.operations))
	copy(operations, e.operations)
	return operations
}

func (e *IsolatedExecutor) createRuntimeRoot(ctx context.Context, step planner.Step) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	statePath := e.storePath()
	if _, err := os.Stat(statePath); err == nil {
		return fmt.Errorf("isolated session runtime state already exists at %s; stop or cleanup the session before starting it again", statePath)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check isolated runtime state: %w", err)
	}
	if err := os.MkdirAll(e.sessionRuntimeRoot, 0o700); err != nil {
		return fmt.Errorf("create isolated runtime root: %w", err)
	}
	if err := os.Chmod(e.sessionRuntimeRoot, 0o700); err != nil {
		return fmt.Errorf("set isolated runtime root permissions: %w", err)
	}
	e.recordRuntime(OperationApply, step.ID, "create "+e.sessionRuntimeRoot)
	return nil
}

func (e *IsolatedExecutor) validatePrerequisites(step planner.Step) error {
	for _, binary := range detailValues(step, "binary") {
		if err := e.validateExecutable(binary); err != nil {
			return err
		}
		e.recordRuntime(OperationApply, step.ID, "found "+binary)
	}
	return nil
}

func (e *IsolatedExecutor) validateExecutable(binary string) error {
	return validateExecutable(e.lookPath, binary)
}

func (e *IsolatedExecutor) createVethPair(ctx context.Context, step planner.Step) error {
	hostVeth := requiredDetailValue(step, "host_veth")
	namespaceVeth := requiredDetailValue(step, "namespace_veth")
	namespace := requiredDetailValue(step, "namespace")
	if err := e.runBuilt(ctx, OperationApply, step.ID, builtCommand(VethCreateCommand(hostVeth, namespaceVeth))); err != nil {
		return err
	}
	return e.runBuilt(ctx, OperationApply, step.ID, builtCommand(LinkSetNetNSCommand(namespaceVeth, namespace)))
}

func (e *IsolatedExecutor) configureHostVeth(ctx context.Context, step planner.Step) error {
	hostVeth := requiredDetailValue(step, "host_veth")
	if err := e.runBuilt(ctx, OperationApply, step.ID, builtCommand(AddressReplaceCommand(hostVeth, requiredDetailValue(step, "host_address")))); err != nil {
		return err
	}
	return e.runBuilt(ctx, OperationApply, step.ID, builtCommand(LinkSetUpCommand(hostVeth)))
}

func (e *IsolatedExecutor) configureNamespaceVeth(ctx context.Context, step planner.Step) error {
	namespace := requiredDetailValue(step, "namespace")
	namespaceVeth := requiredDetailValue(step, "namespace_veth")
	if err := e.runBuilt(ctx, OperationApply, step.ID, builtCommand(NetNSAddressReplaceCommand(namespace, namespaceVeth, requiredDetailValue(step, "namespace_address")))); err != nil {
		return err
	}
	if err := e.runBuilt(ctx, OperationApply, step.ID, builtCommand(NetNSLinkSetUpCommand(namespace, namespaceVeth))); err != nil {
		return err
	}
	return e.runBuilt(ctx, OperationApply, step.ID, builtCommand(NetNSLoopbackSetUpCommand(namespace)))
}

func (e *IsolatedExecutor) configureNamespaceDNS(ctx context.Context, step planner.Step) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	namespace := requiredDetailValue(step, "namespace")
	root := filepath.Join(e.netNSConfigRoot, namespace)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create namespace DNS directory: %w", err)
	}
	var builder strings.Builder
	for _, dns := range detailValues(step, "dns") {
		builder.WriteString("nameserver ")
		builder.WriteString(dns)
		builder.WriteByte('\n')
	}
	path := filepath.Join(root, "resolv.conf")
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("write namespace DNS: %w", err)
	}
	e.recordRuntime(OperationApply, step.ID, "write "+path)
	return nil
}

func (e *IsolatedExecutor) startWSTunnelControl(ctx context.Context, step planner.Step) error {
	command, err := wstunnelControlCommandFromStep(step)
	if err != nil {
		return err
	}
	process, err := e.processRunner.Start(ctx, supervisor.Command{Name: command.Name, Args: command.Args})
	if err != nil {
		return err
	}
	e.wstunnelProcess = process
	e.record(OperationApply, step.ID, command)
	e.recordRuntime(OperationApply, step.ID, fmt.Sprintf("pid %d", process.Info().PID))
	return nil
}

func (e *IsolatedExecutor) applyWireGuardAddresses(ctx context.Context, step planner.Step) error {
	namespace := requiredDetailValue(step, "namespace")
	iface := requiredDetailValue(step, "interface")
	for _, address := range detailValues(step, "address") {
		if err := e.runNamespacedBuilt(ctx, OperationApply, step.ID, namespace, builtCommand(WireGuardAddAddressCommand(iface, address))); err != nil {
			return err
		}
	}
	mtu, err := strconv.Atoi(requiredDetailValue(step, "mtu"))
	if err != nil {
		return fmt.Errorf("wireguard MTU detail is invalid: %w", err)
	}
	if err := e.runNamespacedBuilt(ctx, OperationApply, step.ID, namespace, builtCommand(WireGuardSetMTUCommand(iface, mtu))); err != nil {
		return err
	}
	return e.runNamespacedBuilt(ctx, OperationApply, step.ID, namespace, builtCommand(WireGuardSetUpCommand(iface)))
}

func (e *IsolatedExecutor) applyWireGuardPeer(ctx context.Context, step planner.Step) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	namespace := requiredDetailValue(step, "namespace")
	iface := requiredDetailValue(step, "interface")
	config := wireguard.ConfigFromProfile(e.profile, iface)
	config.Endpoint = requiredDetailValue(step, "endpoint")
	rendered, err := wireguard.RenderSetConf(config)
	if err != nil {
		return err
	}
	path := filepath.Join(e.sessionRuntimeRoot, "wg-setconf.conf")
	if err := os.WriteFile(path, []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("write isolated wireguard config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("set isolated wireguard config permissions: %w", err)
	}
	e.recordRuntime(OperationApply, step.ID, "write redacted WireGuard setconf "+path)
	defer func() {
		if err := os.Remove(path); err == nil {
			e.recordRuntime(OperationApply, step.ID, "remove redacted WireGuard setconf "+path)
		}
	}()
	return e.runNamespacedBuilt(ctx, OperationApply, step.ID, namespace, builtCommand(WireGuardSetConfigCommand(iface, path)))
}

func (e *IsolatedExecutor) applyNamespaceClientRoutes(ctx context.Context, step planner.Step) error {
	return e.replaceNamespaceClientRoutes(ctx, OperationApply, step.ID, requiredDetailValue(step, "namespace"), requiredDetailValue(step, "interface"), detailValues(step, "allowed_ip"))
}

func (e *IsolatedExecutor) applyNamespaceKillSwitch(ctx context.Context, step planner.Step) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ruleset, err := renderNamespaceKillSwitchRules(step)
	if err != nil {
		return err
	}
	path := filepath.Join(e.sessionRuntimeRoot, "namespace-killswitch.nft")
	if err := os.WriteFile(path, []byte(ruleset), 0o600); err != nil {
		return fmt.Errorf("write namespace kill-switch ruleset: %w", err)
	}
	e.recordRuntime(OperationApply, step.ID, "write namespace fail-closed nft ruleset "+path)
	return e.runNamespacedBuilt(ctx, OperationApply, step.ID, requiredDetailValue(step, "namespace"), builtCommand(NftApplyRulesetCommand(path)))
}

func (e *IsolatedExecutor) launchApp(ctx context.Context, step planner.Step) error {
	command, err := isolatedAppLaunchCommandFromStep(step)
	if err != nil {
		return err
	}
	process, err := e.processRunner.Start(ctx, supervisor.Command{Name: command.Name, Args: command.Args})
	if err != nil {
		return err
	}
	e.appProcess = process
	e.record(OperationApply, step.ID, command)
	e.recordRuntime(OperationApply, step.ID, fmt.Sprintf("pid %d", process.Info().PID))
	return nil
}

func (e *IsolatedExecutor) storeRuntimeState(ctx context.Context, step planner.Step) error {
	state, err := truntime.NewStateFromIsolatedAppPlan(e.plan)
	if err != nil {
		return err
	}
	if e.wstunnelProcess != nil {
		info := e.wstunnelProcess.Info()
		state = state.WithWSTunnelProcess(info.PID, info.Command.Argv())
	}
	if e.appProcess != nil {
		info := e.appProcess.Info()
		state = state.WithAppProcess(info.PID, info.Command.Argv())
	}
	if err := e.store.Save(ctx, state); err != nil {
		return err
	}
	e.runtimeState = state
	e.recordRuntime(OperationApply, step.ID, "save "+e.storePath())
	return nil
}

func (e *IsolatedExecutor) readRuntimeState(ctx context.Context, step planner.Step) error {
	state, err := e.store.Load(ctx)
	if err != nil {
		return err
	}
	e.runtimeState = state
	e.recordRuntime(OperationApply, step.ID, "load "+e.storePath())
	return nil
}

func (e *IsolatedExecutor) removeNamespaceClientRoutes(ctx context.Context, step planner.Step) error {
	state, err := e.stateForStop(ctx)
	if err != nil {
		return err
	}
	return e.deleteNamespaceClientRoutes(ctx, OperationApply, step.ID, state.Namespace, state.WireGuardInterface, state.WireGuardAllowedIPs)
}

func (e *IsolatedExecutor) removeNamespaceDNS(ctx context.Context, step planner.Step) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	namespace := requiredDetailValue(step, "namespace")
	if namespace == "" {
		state, err := e.stateForStop(ctx)
		if err != nil {
			return err
		}
		namespace = state.Namespace
	}
	root := filepath.Join(e.netNSConfigRoot, namespace)
	path := filepath.Join(root, "resolv.conf")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove namespace DNS: %w", err)
	}
	if err := os.Remove(root); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove namespace DNS directory: %w", err)
	}
	e.recordRuntime(OperationApply, step.ID, "remove "+path)
	return nil
}

func (e *IsolatedExecutor) cleanupNamespaceDNS(ctx context.Context, step planner.Step) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	namespace := requiredDetailValue(step, "namespace")
	root := filepath.Join(e.netNSConfigRoot, namespace)
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("remove namespace DNS directory: %w", err)
	}
	e.recordRuntime(OperationApply, step.ID, "remove "+root)
	return nil
}

func (e *IsolatedExecutor) clearRuntimeState(ctx context.Context, step planner.Step) error {
	if err := e.store.Clear(ctx); err != nil {
		return err
	}
	if err := e.removeRuntimeRoot(ctx, step); err != nil {
		return err
	}
	e.recordRuntime(OperationApply, step.ID, "clear "+e.storePath())
	return nil
}

func (e *IsolatedExecutor) removeRuntimeRoot(ctx context.Context, step planner.Step) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root := requiredDetailValue(step, "session_runtime_root")
	if root == "" {
		root = e.sessionRuntimeRoot
	}
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("remove isolated runtime root: %w", err)
	}
	e.recordRuntime(OperationApply, step.ID, "remove "+root)
	return nil
}

func (e *IsolatedExecutor) stateForStop(ctx context.Context) (truntime.State, error) {
	if e.runtimeState.Version != 0 {
		return e.runtimeState, nil
	}
	state, err := e.store.Load(ctx)
	if err != nil {
		return truntime.State{}, err
	}
	e.runtimeState = state
	return state, nil
}

func (e *IsolatedExecutor) stopRuntimeProcess(ctx context.Context, stepID string, label string, process *truntime.ProcessState) error {
	if process == nil {
		state, err := e.stateForStop(ctx)
		if err != nil {
			return err
		}
		if label == "app" {
			process = state.AppProcess
		} else {
			process = state.WSTunnelProcess
		}
	}
	if process == nil {
		e.recordRuntime(OperationApply, stepID, "no "+label+" process in runtime state")
		return nil
	}
	if err := e.stopper.StopPID(ctx, process.PID); err != nil {
		return err
	}
	e.recordRuntime(OperationApply, stepID, fmt.Sprintf("stop %s pid %d", label, process.PID))
	return nil
}

func (e *IsolatedExecutor) stopIsolatedApp(ctx context.Context, step planner.Step) error {
	state, err := e.stateForStop(ctx)
	if err != nil {
		return err
	}
	seen := map[int]struct{}{}
	if state.AppProcess != nil {
		if err := e.stopper.StopPID(ctx, state.AppProcess.PID); err != nil {
			return err
		}
		seen[state.AppProcess.PID] = struct{}{}
		e.recordRuntime(OperationApply, step.ID, fmt.Sprintf("stop app pid %d", state.AppProcess.PID))
	}
	command, err := NetNSPidsCommand(state.Namespace)
	if err != nil {
		return err
	}
	e.record(OperationApply, step.ID, command)
	output, err := e.runner.Run(ctx, command)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return fmt.Errorf("run %s: %w", command.String(), err)
		}
		return fmt.Errorf("run %s: %w: %s", command.String(), err, detail)
	}
	for _, token := range strings.Fields(string(output)) {
		pid, err := strconv.Atoi(token)
		if err != nil || pid < 1 {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		if err := e.stopper.StopPID(ctx, pid); err != nil {
			return err
		}
		seen[pid] = struct{}{}
		e.recordRuntime(OperationApply, step.ID, fmt.Sprintf("stop namespace pid %d", pid))
	}
	if len(seen) == 0 {
		e.recordRuntime(OperationApply, step.ID, "no isolated app process in runtime state or namespace")
	}
	return nil
}

func (e *IsolatedExecutor) cleanupIsolatedProcesses(ctx context.Context, step planner.Step) error {
	command, err := NetNSPidsCommand(requiredDetailValue(step, "namespace"))
	if err != nil {
		return err
	}
	e.record(OperationApply, step.ID, command)
	output, err := e.runner.Run(ctx, command)
	if err != nil {
		e.recordRuntime(OperationApply, step.ID, "skip namespace pid cleanup: "+strings.TrimSpace(err.Error()))
		return nil
	}
	stopped := 0
	for _, token := range strings.Fields(string(output)) {
		pid, err := strconv.Atoi(token)
		if err != nil || pid < 1 {
			continue
		}
		if err := e.stopper.StopPID(ctx, pid); err != nil {
			e.recordRuntime(OperationApply, step.ID, fmt.Sprintf("skip pid %d: %s", pid, err))
			continue
		}
		stopped++
		e.recordRuntime(OperationApply, step.ID, fmt.Sprintf("stop namespace pid %d", pid))
	}
	if stopped == 0 {
		e.recordRuntime(OperationApply, step.ID, "no namespace pids stopped")
	}
	return nil
}

func (e *IsolatedExecutor) replaceNamespaceClientRoutes(ctx context.Context, phase OperationPhase, stepID string, namespace string, iface string, allowedIPs []string) error {
	for _, allowedIP := range allowedIPs {
		if err := e.runNamespacedBuilt(ctx, phase, stepID, namespace, builtCommand(WireGuardRouteReplaceCommand(iface, allowedIP))); err != nil {
			return err
		}
	}
	return nil
}

func (e *IsolatedExecutor) deleteNamespaceClientRoutes(ctx context.Context, phase OperationPhase, stepID string, namespace string, iface string, allowedIPs []string) error {
	for _, allowedIP := range allowedIPs {
		if err := e.runNamespacedBuilt(ctx, phase, stepID, namespace, builtCommand(WireGuardRouteDeleteCommand(iface, allowedIP))); err != nil {
			return err
		}
	}
	return nil
}

func builtCommand(command Command, err error) commandBuild {
	return commandBuild{command: command, err: err}
}

func (e *IsolatedExecutor) runBuilt(ctx context.Context, phase OperationPhase, stepID string, built commandBuild) error {
	if built.err != nil {
		return built.err
	}
	return e.run(ctx, phase, stepID, built.command)
}

func (e *IsolatedExecutor) runNamespacedBuilt(ctx context.Context, phase OperationPhase, stepID string, namespace string, built commandBuild) error {
	if built.err != nil {
		return built.err
	}
	return e.runNamespaced(ctx, phase, stepID, namespace, built.command)
}

func (e *IsolatedExecutor) runNamespaced(ctx context.Context, phase OperationPhase, stepID string, namespace string, command Command) error {
	namespaced, err := NetNSExecCommand(namespace, command)
	if err != nil {
		return err
	}
	return e.run(ctx, phase, stepID, namespaced)
}

func (e *IsolatedExecutor) runBestEffortBuilt(ctx context.Context, phase OperationPhase, stepID string, built commandBuild) error {
	if built.err != nil {
		return built.err
	}
	e.runBestEffort(ctx, phase, stepID, built.command)
	return nil
}

func (e *IsolatedExecutor) runNamespacedBestEffortBuilt(ctx context.Context, phase OperationPhase, stepID string, namespace string, built commandBuild) error {
	if built.err != nil {
		return built.err
	}
	return e.runNamespacedBestEffort(ctx, phase, stepID, namespace, built.command)
}

func (e *IsolatedExecutor) runNamespacedBestEffort(ctx context.Context, phase OperationPhase, stepID string, namespace string, command Command) error {
	namespaced, err := NetNSExecCommand(namespace, command)
	if err != nil {
		return err
	}
	e.runBestEffort(ctx, phase, stepID, namespaced)
	return nil
}

func (e *IsolatedExecutor) runBestEffort(ctx context.Context, phase OperationPhase, stepID string, command Command) {
	e.record(phase, stepID, command)
	output, err := e.runner.Run(ctx, command)
	if err == nil {
		return
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		detail = strings.TrimSpace(err.Error())
	}
	e.recordRuntime(phase, stepID, "ignore cleanup error from "+command.String()+": "+detail)
}

func (e *IsolatedExecutor) run(ctx context.Context, phase OperationPhase, stepID string, command Command) error {
	e.record(phase, stepID, command)
	output, err := e.runner.Run(ctx, command)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return fmt.Errorf("run %s: %w", command.String(), err)
		}
		return fmt.Errorf("run %s: %w: %s", command.String(), err, detail)
	}
	return nil
}

func (e *IsolatedExecutor) record(phase OperationPhase, stepID string, command Command) {
	e.operations = append(e.operations, Operation{
		Phase:   phase,
		StepID:  stepID,
		Command: &command,
	})
}

func (e *IsolatedExecutor) recordRuntime(phase OperationPhase, stepID string, runtime string) {
	e.operations = append(e.operations, Operation{
		Phase:   phase,
		StepID:  stepID,
		Runtime: runtime,
	})
}

func (e *IsolatedExecutor) storePath() string {
	path, err := e.store.Path()
	if err != nil {
		return e.store.Root
	}
	return path
}

func renderNamespaceKillSwitchRules(step planner.Step) (string, error) {
	iface := requiredDetailValue(step, "interface")
	namespaceVeth := requiredDetailValue(step, "namespace_veth")
	endpoint := requiredDetailValue(step, "allowed_control_endpoint")
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse control endpoint: %w", err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("parse control endpoint port: %w", err)
	}
	if portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("control endpoint port must be in 1..65535")
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return "", fmt.Errorf("parse control endpoint address: %w", err)
	}

	var builder strings.Builder
	builder.WriteString("flush ruleset\n")
	builder.WriteString("table inet brb_isolated {\n")
	builder.WriteString("  chain output {\n")
	builder.WriteString("    type filter hook output priority 0; policy drop;\n")
	builder.WriteString("    oifname \"lo\" accept\n")
	builder.WriteString("    oifname \"")
	builder.WriteString(iface)
	builder.WriteString("\" accept\n")
	builder.WriteString("    oifname \"")
	builder.WriteString(namespaceVeth)
	builder.WriteString("\" ")
	if addr.Is4() {
		builder.WriteString("ip daddr ")
	} else {
		builder.WriteString("ip6 daddr ")
	}
	builder.WriteString(addr.String())
	builder.WriteString(" udp dport ")
	builder.WriteString(strconv.Itoa(portNumber))
	builder.WriteString(" accept\n")
	builder.WriteString("  }\n")
	builder.WriteString("}\n")
	return builder.String(), nil
}
