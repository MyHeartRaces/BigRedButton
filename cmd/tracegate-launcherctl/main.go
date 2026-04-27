package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	stdruntime "runtime"

	"github.com/tracegate/tracegate-launcher/internal/engine"
	"github.com/tracegate/tracegate-launcher/internal/planner"
	platformlinux "github.com/tracegate/tracegate-launcher/internal/platform/linux"
	"github.com/tracegate/tracegate-launcher/internal/profile"
	truntime "github.com/tracegate/tracegate-launcher/internal/runtime"
	"github.com/tracegate/tracegate-launcher/internal/status"
	"github.com/tracegate/tracegate-launcher/internal/supervisor"
)

var currentGOOS = stdruntime.GOOS

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "validate-profile":
		return validateProfile(args[1:], stdout, stderr)
	case "plan-connect":
		return planConnect(args[1:], stdout, stderr)
	case "plan-disconnect":
		return planDisconnect(args[1:], stdout, stderr)
	case "status":
		return printStatus(args[1:], stdout, stderr)
	case "linux-dry-run-connect":
		return linuxDryRunConnect(args[1:], stdout, stderr)
	case "linux-dry-run-disconnect":
		return linuxDryRunDisconnect(args[1:], stdout, stderr)
	case "linux-connect":
		return linuxConnect(args[1:], stdout, stderr)
	case "linux-disconnect":
		return linuxDisconnect(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func validateProfile(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate-profile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: tracegate-launcherctl validate-profile [-json] <profile.json>")
		return 2
	}

	path := fs.Arg(0)
	config, err := profile.LoadFile(path)
	if err != nil {
		if validationErr, ok := profile.AsValidationError(err); ok {
			if *jsonOutput {
				writeJSON(stdout, map[string]any{
					"valid":  false,
					"errors": validationErr.Problems,
				})
			} else {
				fmt.Fprintln(stderr, "profile invalid:")
				for _, problem := range validationErr.Problems {
					fmt.Fprintf(stderr, "- %s\n", problem)
				}
			}
			return 1
		}
		fmt.Fprintf(stderr, "read profile: %v\n", err)
		return 1
	}

	summary := config.Summary()
	if *jsonOutput {
		writeJSON(stdout, map[string]any{
			"valid":   true,
			"summary": summary,
		})
		return 0
	}

	fmt.Fprintf(stdout, "profile valid: %s\n", path)
	fmt.Fprintf(stdout, "profile: %s\n", summary.Profile)
	fmt.Fprintf(stdout, "server: %s:%d\n", summary.Server, summary.Port)
	fmt.Fprintf(stdout, "wstunnel: %s\n", summary.WSTunnelURL)
	fmt.Fprintf(stdout, "local UDP: %s\n", summary.LocalUDPListen)
	fmt.Fprintf(stdout, "addresses: %v\n", summary.Addresses)
	fmt.Fprintf(stdout, "allowed IPs: %v\n", summary.AllowedIPs)
	fmt.Fprintf(stdout, "mtu: %d\n", summary.MTU)
	fmt.Fprintf(stdout, "persistent keepalive: %d\n", summary.PersistentKeepalive)
	fmt.Fprintf(stdout, "preshared key: %t\n", summary.HasPresharedKey)
	fmt.Fprintf(stdout, "fingerprint: %s\n", summary.Fingerprint)
	return 0
}

func planConnect(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan-connect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	options := planConnectFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: tracegate-launcherctl plan-connect [-json] [-endpoint-ip ip[,ip]] <profile.json>")
		return 2
	}

	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		printProfileError(err, stderr, *jsonOutput, stdout)
		return 1
	}
	plan, err := planner.Connect(config, planner.Options{
		EndpointIPs:        csvOption(*options.endpointIPs),
		DefaultGateway:     *options.defaultGateway,
		DefaultInterface:   *options.defaultInterface,
		WSTunnelBinary:     *options.wstunnelBinary,
		WireGuardInterface: *options.wireguardInterface,
		RuntimeRoot:        *options.runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build connect plan: %v\n", err)
		return 1
	}
	printPlan(plan, *jsonOutput, stdout)
	return 0
}

func printStatus(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	runtimeRoot := fs.String("runtime-root", planner.DefaultRuntimeRoot, "launcher runtime state root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: tracegate-launcherctl status [-json] [-runtime-root path]")
		return 2
	}

	snapshot := status.FromStore(context.Background(), truntime.Store{Root: *runtimeRoot})
	if *jsonOutput {
		writeJSON(stdout, snapshot)
		return 0
	}
	fmt.Fprintf(stdout, "state: %s\n", snapshot.State)
	fmt.Fprintf(stdout, "runtime root: %s\n", snapshot.RuntimeRoot)
	if snapshot.Active != nil {
		fmt.Fprintf(stdout, "profile fingerprint: %s\n", snapshot.Active.ProfileFingerprint)
		fmt.Fprintf(stdout, "wireguard interface: %s\n", snapshot.Active.WireGuardInterface)
		if snapshot.Active.WSTunnelProcess != nil {
			fmt.Fprintf(stdout, "wstunnel pid: %d\n", snapshot.Active.WSTunnelProcess.PID)
		}
	}
	if snapshot.Error != "" {
		fmt.Fprintf(stdout, "error: %s\n", snapshot.Error)
		return 1
	}
	return 0
}

func linuxDryRunConnect(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-dry-run-connect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	discoverRoutes := fs.Bool("discover-routes", false, "run read-only Linux route discovery during dry-run")
	persistRuntimeState := fs.Bool("persist-runtime-state", false, "write runtime state during dry-run")
	options := planConnectFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: tracegate-launcherctl linux-dry-run-connect [-json] [-discover-routes] [-persist-runtime-state] [-endpoint-ip ip[,ip]] <profile.json>")
		return 2
	}

	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		printProfileError(err, stderr, *jsonOutput, stdout)
		return 1
	}
	plan, err := planner.Connect(config, planner.Options{
		EndpointIPs:        csvOption(*options.endpointIPs),
		DefaultGateway:     *options.defaultGateway,
		DefaultInterface:   *options.defaultInterface,
		WSTunnelBinary:     *options.wstunnelBinary,
		WireGuardInterface: *options.wireguardInterface,
		RuntimeRoot:        *options.runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build connect plan: %v\n", err)
		return 1
	}
	executor, err := platformlinux.NewDryRunExecutorWithOptions(plan, platformlinux.DryRunOptions{
		ReadOnlyDiscovery: *discoverRoutes,
		PersistRuntime:    *persistRuntimeState,
		RuntimeRoot:       plan.RuntimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build linux dry-run executor: %v\n", err)
		return 1
	}
	result := engine.New(executor).Run(context.Background(), plan)
	output := linuxDryRunOutput{
		Plan:       plan,
		Result:     result,
		Operations: executor.Operations(),
	}
	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printPlan(plan, false, stdout)
		printLinuxDryRun(output, stdout)
	}
	if result.State != engine.StateConnected {
		return 1
	}
	return 0
}

func linuxDryRunDisconnect(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-dry-run-disconnect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	persistRuntimeState := fs.Bool("persist-runtime-state", false, "clear runtime state during dry-run")
	wireguardInterface := fs.String("wireguard-interface", "", "WireGuard interface name")
	runtimeRoot := fs.String("runtime-root", "", "launcher runtime state root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: tracegate-launcherctl linux-dry-run-disconnect [-json] [-persist-runtime-state] [-wireguard-interface name] [-runtime-root path]")
		return 2
	}

	plan, err := planner.Disconnect(planner.Options{
		WireGuardInterface: *wireguardInterface,
		RuntimeRoot:        *runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build disconnect plan: %v\n", err)
		return 1
	}
	executor, err := platformlinux.NewDryRunExecutorWithOptions(plan, platformlinux.DryRunOptions{
		PersistRuntime: *persistRuntimeState,
		RuntimeRoot:    plan.RuntimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build linux dry-run executor: %v\n", err)
		return 1
	}
	result := engine.New(executor).Run(context.Background(), plan)
	output := linuxDryRunOutput{
		Plan:       plan,
		Result:     result,
		Operations: executor.Operations(),
	}
	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printPlan(plan, false, stdout)
		printLinuxDryRun(output, stdout)
	}
	if result.State != engine.StateIdle {
		return 1
	}
	return 0
}

func linuxConnect(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-connect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	confirmed := fs.Bool("yes", false, "confirm this command may change Linux networking state")
	options := planConnectFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: tracegate-launcherctl linux-connect -yes [-json] [-endpoint-ip ip[,ip]] <profile.json>")
		return 2
	}
	if !*confirmed {
		fmt.Fprintln(stderr, "linux-connect requires -yes because it changes routes, processes and WireGuard state")
		return 2
	}
	if currentGOOS != "linux" {
		fmt.Fprintf(stderr, "linux-connect can only run on Linux; current OS is %s\n", currentGOOS)
		return 1
	}

	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		printProfileError(err, stderr, *jsonOutput, stdout)
		return 1
	}
	plan, err := planner.Connect(config, planner.Options{
		EndpointIPs:        csvOption(*options.endpointIPs),
		DefaultGateway:     *options.defaultGateway,
		DefaultInterface:   *options.defaultInterface,
		WSTunnelBinary:     *options.wstunnelBinary,
		WireGuardInterface: *options.wireguardInterface,
		RuntimeRoot:        *options.runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build connect plan: %v\n", err)
		return 1
	}
	if len(plan.EndpointIPs) == 0 {
		fmt.Fprintln(stderr, "linux-connect requires at least one -endpoint-ip for the WSTunnel server")
		return 2
	}
	executor, err := platformlinux.NewLifecycleExecutor(platformlinux.LifecycleExecutorOptions{
		Plan:           plan,
		Profile:        config,
		WSTunnelBinary: *options.wstunnelBinary,
		RuntimeRoot:    plan.RuntimeRoot,
		WireGuardIface: plan.WireGuardInterface,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build linux lifecycle executor: %v\n", err)
		return 1
	}
	result := engine.New(executor).Run(context.Background(), plan)
	output := linuxLifecycleOutput{
		Plan:                plan,
		Result:              result,
		RouteOperations:     executor.RouteOperations(),
		WSTunnelOperations:  executor.WSTunnelOperations(),
		WireGuardOperations: executor.WireGuardOperations(),
	}
	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printPlan(plan, false, stdout)
		printLinuxLifecycle(output, stdout)
	}
	if result.State != engine.StateConnected {
		return 1
	}
	return 0
}

func linuxDisconnect(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-disconnect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	confirmed := fs.Bool("yes", false, "confirm this command may change Linux networking state")
	wireguardInterface := fs.String("wireguard-interface", "", "WireGuard interface name")
	runtimeRoot := fs.String("runtime-root", "", "launcher runtime state root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: tracegate-launcherctl linux-disconnect -yes [-json] [-runtime-root path] <profile.json>")
		return 2
	}
	if !*confirmed {
		fmt.Fprintln(stderr, "linux-disconnect requires -yes because it changes processes, routes and WireGuard state")
		return 2
	}
	if currentGOOS != "linux" {
		fmt.Fprintf(stderr, "linux-disconnect can only run on Linux; current OS is %s\n", currentGOOS)
		return 1
	}

	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		printProfileError(err, stderr, *jsonOutput, stdout)
		return 1
	}
	plan, err := planner.Disconnect(planner.Options{
		WireGuardInterface: *wireguardInterface,
		RuntimeRoot:        *runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build disconnect plan: %v\n", err)
		return 1
	}
	executor, err := platformlinux.NewLifecycleExecutor(platformlinux.LifecycleExecutorOptions{
		Plan:           plan,
		Profile:        config,
		RuntimeRoot:    plan.RuntimeRoot,
		WireGuardIface: plan.WireGuardInterface,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build linux lifecycle executor: %v\n", err)
		return 1
	}
	result := engine.New(executor).Run(context.Background(), plan)
	output := linuxLifecycleOutput{
		Plan:                plan,
		Result:              result,
		RouteOperations:     executor.RouteOperations(),
		WSTunnelOperations:  executor.WSTunnelOperations(),
		WireGuardOperations: executor.WireGuardOperations(),
	}
	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printPlan(plan, false, stdout)
		printLinuxLifecycle(output, stdout)
	}
	if result.State != engine.StateIdle {
		return 1
	}
	return 0
}

func planDisconnect(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan-disconnect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	wireguardInterface := fs.String("wireguard-interface", "", "WireGuard interface name")
	runtimeRoot := fs.String("runtime-root", "", "launcher runtime state root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: tracegate-launcherctl plan-disconnect [-json]")
		return 2
	}

	plan, err := planner.Disconnect(planner.Options{
		WireGuardInterface: *wireguardInterface,
		RuntimeRoot:        *runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build disconnect plan: %v\n", err)
		return 1
	}
	printPlan(plan, *jsonOutput, stdout)
	return 0
}

func writeJSON(w io.Writer, payload any) {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

type connectFlagValues struct {
	endpointIPs        *string
	defaultGateway     *string
	defaultInterface   *string
	wstunnelBinary     *string
	wireguardInterface *string
	runtimeRoot        *string
}

type linuxDryRunOutput struct {
	Plan       planner.Plan              `json:"plan"`
	Result     engine.Result             `json:"result"`
	Operations []platformlinux.Operation `json:"operations"`
}

type linuxLifecycleOutput struct {
	Plan                planner.Plan                   `json:"plan"`
	Result              engine.Result                  `json:"result"`
	RouteOperations     []platformlinux.Operation      `json:"route_operations,omitempty"`
	WSTunnelOperations  []supervisor.WSTunnelOperation `json:"wstunnel_operations,omitempty"`
	WireGuardOperations []platformlinux.Operation      `json:"wireguard_operations,omitempty"`
}

func planConnectFlags(fs *flag.FlagSet) connectFlagValues {
	return connectFlagValues{
		endpointIPs:        fs.String("endpoint-ip", "", "comma-separated resolved WSTunnel endpoint IPs"),
		defaultGateway:     fs.String("default-gateway", "", "pre-tunnel default gateway for route exclusion"),
		defaultInterface:   fs.String("default-interface", "", "pre-tunnel default interface for route exclusion"),
		wstunnelBinary:     fs.String("wstunnel-binary", "", "WSTunnel binary path/name"),
		wireguardInterface: fs.String("wireguard-interface", "", "WireGuard interface name"),
		runtimeRoot:        fs.String("runtime-root", "", "launcher runtime state root"),
	}
}

func printProfileError(err error, stderr io.Writer, jsonOutput bool, stdout io.Writer) {
	if validationErr, ok := profile.AsValidationError(err); ok {
		if jsonOutput {
			writeJSON(stdout, map[string]any{
				"valid":  false,
				"errors": validationErr.Problems,
			})
		} else {
			fmt.Fprintln(stderr, "profile invalid:")
			for _, problem := range validationErr.Problems {
				fmt.Fprintf(stderr, "- %s\n", problem)
			}
		}
		return
	}
	fmt.Fprintf(stderr, "read profile: %v\n", err)
}

func printPlan(plan planner.Plan, jsonOutput bool, stdout io.Writer) {
	if jsonOutput {
		writeJSON(stdout, plan)
		return
	}
	fmt.Fprintf(stdout, "%s plan\n", plan.Kind)
	if plan.Profile != "" {
		fmt.Fprintf(stdout, "profile: %s\n", plan.Profile)
		fmt.Fprintf(stdout, "fingerprint: %s\n", plan.ProfileFingerprint)
	}
	if plan.WSTunnelURL != "" {
		fmt.Fprintf(stdout, "wstunnel: %s\n", plan.WSTunnelURL)
	}
	if plan.WireGuardInterface != "" {
		fmt.Fprintf(stdout, "wireguard interface: %s\n", plan.WireGuardInterface)
	}
	if len(plan.EndpointIPs) > 0 {
		fmt.Fprintf(stdout, "endpoint IPs: %v\n", plan.EndpointIPs)
	}
	if len(plan.Warnings) > 0 {
		fmt.Fprintln(stdout, "warnings:")
		for _, warning := range plan.Warnings {
			fmt.Fprintf(stdout, "- %s\n", warning)
		}
	}
	fmt.Fprintln(stdout, "steps:")
	for index, step := range plan.Steps {
		privileged := ""
		if step.RequiresPrivilege {
			privileged = " [privileged]"
		}
		skipped := ""
		if step.SkippedUntilApply {
			skipped = " [apply-time]"
		}
		fmt.Fprintf(stdout, "%d. %s%s%s\n", index+1, step.Action, privileged, skipped)
		for _, detail := range step.Details {
			fmt.Fprintf(stdout, "   - %s\n", detail)
		}
		if len(step.Rollback) > 0 {
			fmt.Fprintf(stdout, "   rollback: %v\n", step.Rollback)
		}
	}
}

func printLinuxDryRun(output linuxDryRunOutput, stdout io.Writer) {
	fmt.Fprintf(stdout, "engine state: %s\n", output.Result.State)
	if output.Result.Error != "" {
		fmt.Fprintf(stdout, "engine error: %s\n", output.Result.Error)
	}
	if len(output.Operations) == 0 {
		fmt.Fprintln(stdout, "linux dry-run commands: []")
		return
	}
	fmt.Fprintln(stdout, "linux dry-run commands:")
	for _, operation := range output.Operations {
		if operation.Runtime != "" {
			fmt.Fprintf(stdout, "- %s %s: %s\n", operation.Phase, operation.StepID, operation.Runtime)
			continue
		}
		if operation.Command == nil {
			continue
		}
		fmt.Fprintf(stdout, "- %s %s: %s\n", operation.Phase, operation.StepID, operation.Command.String())
	}
}

func printLinuxLifecycle(output linuxLifecycleOutput, stdout io.Writer) {
	fmt.Fprintf(stdout, "engine state: %s\n", output.Result.State)
	if output.Result.Error != "" {
		fmt.Fprintf(stdout, "engine error: %s\n", output.Result.Error)
	}
	if output.Result.RollbackError != "" {
		fmt.Fprintf(stdout, "rollback error: %s\n", output.Result.RollbackError)
	}
	printLinuxOperations(stdout, "route operations", output.RouteOperations)
	printWSTunnelOperations(stdout, output.WSTunnelOperations)
	printLinuxOperations(stdout, "wireguard operations", output.WireGuardOperations)
}

func printLinuxOperations(stdout io.Writer, title string, operations []platformlinux.Operation) {
	if len(operations) == 0 {
		return
	}
	fmt.Fprintf(stdout, "%s:\n", title)
	for _, operation := range operations {
		if operation.Runtime != "" {
			fmt.Fprintf(stdout, "- %s %s: %s\n", operation.Phase, operation.StepID, operation.Runtime)
			continue
		}
		if operation.Command == nil {
			continue
		}
		fmt.Fprintf(stdout, "- %s %s: %s\n", operation.Phase, operation.StepID, operation.Command.String())
	}
}

func printWSTunnelOperations(stdout io.Writer, operations []supervisor.WSTunnelOperation) {
	if len(operations) == 0 {
		return
	}
	fmt.Fprintln(stdout, "wstunnel operations:")
	for _, operation := range operations {
		if operation.Command != nil {
			fmt.Fprintf(stdout, "- %s %s: %s\n", operation.Phase, operation.StepID, operation.Command.String())
			continue
		}
		if operation.Process != nil {
			fmt.Fprintf(stdout, "- %s %s: pid=%d\n", operation.Phase, operation.StepID, operation.Process.PID)
		}
	}
}

func csvOption(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: tracegate-launcherctl <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  validate-profile [-json] <profile.json>")
	fmt.Fprintln(w, "  plan-connect [-json] [-endpoint-ip ip[,ip]] <profile.json>")
	fmt.Fprintln(w, "  plan-disconnect [-json]")
	fmt.Fprintln(w, "  status [-json] [-runtime-root path]")
	fmt.Fprintln(w, "  linux-dry-run-connect [-json] [-discover-routes] [-persist-runtime-state] [-endpoint-ip ip[,ip]] <profile.json>")
	fmt.Fprintln(w, "  linux-dry-run-disconnect [-json] [-persist-runtime-state] [-wireguard-interface name] [-runtime-root path]")
	fmt.Fprintln(w, "  linux-connect -yes [-json] [-endpoint-ip ip[,ip]] <profile.json>")
	fmt.Fprintln(w, "  linux-disconnect -yes [-json] [-runtime-root path] <profile.json>")
}
