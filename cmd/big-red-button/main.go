package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	userpkg "os/user"
	"path/filepath"
	stdruntime "runtime"
	"sort"
	"strings"
	"time"

	"github.com/MyHeartRaces/BigRedButton/internal/buildinfo"
	"github.com/MyHeartRaces/BigRedButton/internal/engine"
	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	platformlinux "github.com/MyHeartRaces/BigRedButton/internal/platform/linux"
	"github.com/MyHeartRaces/BigRedButton/internal/profile"
	"github.com/MyHeartRaces/BigRedButton/internal/routes"
	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
	"github.com/MyHeartRaces/BigRedButton/internal/status"
	"github.com/MyHeartRaces/BigRedButton/internal/supervisor"
)

var currentGOOS = stdruntime.GOOS
var currentEUID = os.Geteuid
var lookupIPAddr = net.DefaultResolver.LookupIPAddr
var executableLookPath = exec.LookPath
var preflightCommandRunner platformlinux.CommandRunner = platformlinux.ExecRunner{}
var monitorPIDExists = supervisor.PIDExists

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "version", "--version", "-v":
		return printVersion(stdout)
	case "validate-profile":
		return validateProfile(args[1:], stdout, stderr)
	case "plan-connect":
		return planConnect(args[1:], stdout, stderr)
	case "plan-isolated-app":
		return planIsolatedApp(args[1:], stdout, stderr)
	case "plan-isolated-stop":
		return planIsolatedStop(args[1:], stdout, stderr)
	case "plan-isolated-cleanup":
		return planIsolatedCleanup(args[1:], stdout, stderr)
	case "plan-disconnect":
		return planDisconnect(args[1:], stdout, stderr)
	case "status":
		return printStatus(args[1:], stdout, stderr)
	case "isolated-status":
		return printIsolatedStatus(args[1:], stdout, stderr)
	case "isolated-sessions":
		return printIsolatedSessions(args[1:], stdout, stderr)
	case "diagnostics":
		return printDiagnostics(args[1:], stdout, stderr)
	case "diagnostics-bundle":
		return writeDiagnosticsBundleCommand(args[1:], stdout, stderr)
	case "linux-dry-run-connect":
		return linuxDryRunConnect(args[1:], stdout, stderr)
	case "linux-preflight":
		return linuxPreflight(args[1:], stdout, stderr)
	case "linux-preflight-isolated-app":
		return linuxPreflightIsolatedApp(args[1:], stdout, stderr)
	case "linux-dry-run-isolated-app":
		return linuxDryRunIsolatedApp(args[1:], stdout, stderr)
	case "linux-dry-run-disconnect":
		return linuxDryRunDisconnect(args[1:], stdout, stderr)
	case "linux-connect":
		return linuxConnect(args[1:], stdout, stderr)
	case "linux-isolated-app":
		return linuxIsolatedApp(args[1:], stdout, stderr)
	case "linux-monitor-isolated-app":
		return linuxMonitorIsolatedApp(args[1:], stdout, stderr)
	case "linux-stop-isolated-app":
		return linuxStopIsolatedApp(args[1:], stdout, stderr)
	case "linux-cleanup-isolated-app":
		return linuxCleanupIsolatedApp(args[1:], stdout, stderr)
	case "linux-recover-isolated-sessions":
		return linuxRecoverIsolatedSessions(args[1:], stdout, stderr)
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

func printVersion(stdout io.Writer) int {
	info := buildinfo.Current()
	fmt.Fprintf(stdout, "Big Red Button %s\n", buildinfo.DisplayVersion())
	if info.Commit != "" {
		fmt.Fprintf(stdout, "commit: %s\n", info.Commit)
	}
	if info.Date != "" {
		fmt.Fprintf(stdout, "built: %s\n", info.Date)
	}
	return 0
}

func validateProfile(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate-profile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: big-red-button validate-profile [-json] <profile.json>")
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
		fmt.Fprintln(stderr, "usage: big-red-button plan-connect [-json] [-endpoint-ip ip[,ip]] <profile.json>")
		return 2
	}

	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		printProfileError(err, stderr, *jsonOutput, stdout)
		return 1
	}
	plan, err := buildConnectPlan(config, options, csvOption(*options.endpointIPs))
	if err != nil {
		fmt.Fprintf(stderr, "build connect plan: %v\n", err)
		return 1
	}
	printPlan(plan, *jsonOutput, stdout)
	return 0
}

func planIsolatedApp(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan-isolated-app", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	options := isolatedAppFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(stderr, "usage: big-red-button plan-isolated-app [-json] [-session-id uuid] [-app-id uuid] [-dns ip[,ip]] [-app-uid uid -app-gid gid] [-app-env KEY=value] <profile.json> -- <command> [args...]")
		return 2
	}

	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		printProfileError(err, stderr, *jsonOutput, stdout)
		return 1
	}
	isolatedOptions, err := withDefaultSessionID(isolatedAppOptionsFromFlags(options, isolatedAppCommandArgs(fs.Args())))
	if err != nil {
		fmt.Fprintf(stderr, "generate isolated session ID: %v\n", err)
		return 1
	}
	plan, err := planner.IsolatedAppTunnel(config, isolatedOptions)
	if err != nil {
		fmt.Fprintf(stderr, "build isolated app plan: %v\n", err)
		return 1
	}
	printPlan(plan, *jsonOutput, stdout)
	return 0
}

func planIsolatedStop(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan-isolated-stop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	options := isolatedStopFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: big-red-button plan-isolated-stop [-json] -session-id uuid [-runtime-root path]")
		return 2
	}
	plan, err := planner.IsolatedAppStop(planner.IsolatedAppStopOptions{
		SessionID:   *options.sessionID,
		RuntimeRoot: *options.runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build isolated stop plan: %v\n", err)
		return 1
	}
	printPlan(plan, *jsonOutput, stdout)
	return 0
}

func planIsolatedCleanup(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan-isolated-cleanup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	options := isolatedStopFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: big-red-button plan-isolated-cleanup [-json] -session-id uuid [-runtime-root path]")
		return 2
	}
	plan, err := planner.IsolatedAppCleanup(planner.IsolatedAppStopOptions{
		SessionID:   *options.sessionID,
		RuntimeRoot: *options.runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build isolated cleanup plan: %v\n", err)
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
		fmt.Fprintln(stderr, "usage: big-red-button status [-json] [-runtime-root path]")
		return 2
	}

	snapshot := status.FromStore(context.Background(), truntime.Store{Root: *runtimeRoot})
	if *jsonOutput {
		writeJSON(stdout, snapshot)
		return 0
	}
	printStatusSnapshot(stdout, snapshot)
	if snapshot.Error != "" {
		return 1
	}
	return 0
}

func printIsolatedStatus(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("isolated-status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	options := isolatedStopFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: big-red-button isolated-status [-json] -session-id uuid [-runtime-root path]")
		return 2
	}
	plan, err := planner.IsolatedAppStop(planner.IsolatedAppStopOptions{
		SessionID:   *options.sessionID,
		RuntimeRoot: *options.runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build isolated status target: %v\n", err)
		return 1
	}
	snapshot := status.FromStore(context.Background(), truntime.Store{
		Root: filepath.Join(plan.RuntimeRoot, planner.DefaultIsolatedRuntimeSubdir, plan.SessionID),
	})
	if *jsonOutput {
		writeJSON(stdout, snapshot)
		return 0
	}
	printStatusSnapshot(stdout, snapshot)
	if snapshot.Error != "" {
		return 1
	}
	return 0
}

func printIsolatedSessions(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("isolated-sessions", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	runtimeRoot := fs.String("runtime-root", planner.DefaultRuntimeRoot, "launcher runtime state root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: big-red-button isolated-sessions [-json] [-runtime-root path]")
		return 2
	}
	sessions, err := status.IsolatedSessions(context.Background(), *runtimeRoot)
	if err != nil {
		if *jsonOutput {
			writeJSON(stdout, map[string]any{
				"sessions": []status.IsolatedSessionSnapshot{},
				"error":    err.Error(),
			})
		} else {
			fmt.Fprintf(stderr, "list isolated sessions: %v\n", err)
		}
		return 1
	}
	if *jsonOutput {
		writeJSON(stdout, map[string]any{"sessions": sessions})
		return 0
	}
	printIsolatedSessionList(stdout, sessions, *runtimeRoot)
	return 0
}

func printDiagnostics(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("diagnostics", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	runtimeRoot := fs.String("runtime-root", planner.DefaultRuntimeRoot, "launcher runtime state root")
	profilePath := fs.String("profile", "", "optional profile path to summarize")
	wstunnelBinary := fs.String("wstunnel-binary", "", "WSTunnel binary path/name for host checks")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: big-red-button diagnostics [-json] [-runtime-root path] [-profile profile.json] [-wstunnel-binary path]")
		return 2
	}

	output := collectDiagnostics(*runtimeRoot, *profilePath, *wstunnelBinary)

	if *jsonOutput {
		writeJSON(stdout, output)
		return diagnosticsExitCode(output)
	}
	printDiagnosticsOutput(stdout, output)
	return diagnosticsExitCode(output)
}

func writeDiagnosticsBundleCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("diagnostics-bundle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	runtimeRoot := fs.String("runtime-root", planner.DefaultRuntimeRoot, "launcher runtime state root")
	profilePath := fs.String("profile", "", "optional profile path to summarize")
	wstunnelBinary := fs.String("wstunnel-binary", "", "WSTunnel binary path/name for host checks")
	outputPath := fs.String("output", "", "output .tar.gz path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: big-red-button diagnostics-bundle [-runtime-root path] [-profile profile.json] [-wstunnel-binary path] [-output path.tar.gz]")
		return 2
	}

	output := collectDiagnostics(*runtimeRoot, *profilePath, *wstunnelBinary)
	path := strings.TrimSpace(*outputPath)
	if path == "" {
		path = defaultDiagnosticsBundlePath(output.GeneratedAt)
	}
	if err := writeDiagnosticsBundle(path, output); err != nil {
		fmt.Fprintf(stderr, "write diagnostics bundle: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "diagnostics bundle: %s\n", path)
	return 0
}

func collectDiagnostics(runtimeRoot string, profilePath string, wstunnelBinary string) diagnosticsOutput {
	runtimeRoot = strings.TrimSpace(runtimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = planner.DefaultRuntimeRoot
	}
	output := diagnosticsOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Version:     buildinfo.Current(),
		OS:          currentGOOS,
		Host:        collectHostDiagnostics(wstunnelBinary),
		RuntimeRoot: runtimeRoot,
		Runtime:     status.FromStore(context.Background(), truntime.Store{Root: runtimeRoot}),
		ProfilePath: strings.TrimSpace(profilePath),
	}
	sessions, err := status.IsolatedSessions(context.Background(), runtimeRoot)
	if err != nil {
		output.Runtime.Error = appendError(output.Runtime.Error, "isolated sessions: "+err.Error())
	} else {
		output.IsolatedSessions = sessions
	}
	if output.ProfilePath != "" {
		config, err := profile.LoadFile(output.ProfilePath)
		if err != nil {
			output.ProfileError = err.Error()
		} else {
			summary := config.Summary()
			output.Profile = &summary
		}
	}
	return output
}

func collectHostDiagnostics(wstunnelBinary string) diagnosticsHost {
	host := diagnosticsHost{EffectiveUID: currentEUID()}
	if executable, err := os.Executable(); err == nil {
		host.Executable = executable
	}
	if currentGOOS != "linux" {
		return host
	}

	binaries := []string{"ip", "wg"}
	wstunnelBinary = strings.TrimSpace(wstunnelBinary)
	if wstunnelBinary == "" {
		wstunnelBinary = planner.DefaultWSTunnelBinary
	}
	binaries = append(binaries, wstunnelBinary, "resolvectl", "nft", "setpriv")
	if currentEUID() != 0 {
		binaries = append(binaries, "pkexec")
	}
	for _, binary := range binaries {
		host.Checks = append(host.Checks, executableCheck("binary "+binary, binary))
	}
	return host
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
		fmt.Fprintln(stderr, "usage: big-red-button linux-dry-run-connect [-json] [-discover-routes] [-persist-runtime-state] [-endpoint-ip ip[,ip]] <profile.json>")
		return 2
	}

	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		printProfileError(err, stderr, *jsonOutput, stdout)
		return 1
	}
	endpointIPs := csvOption(*options.endpointIPs)
	if len(endpointIPs) == 0 {
		endpointIPs, err = resolveEndpointIPs(context.Background(), config.WSTunnelHost)
		if err != nil {
			fmt.Fprintf(stderr, "resolve WSTunnel endpoint: %v\n", err)
			return 1
		}
	}
	plan, err := buildConnectPlan(config, options, endpointIPs)
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

func linuxPreflight(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-preflight", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	discoverRoutes := fs.Bool("discover-routes", false, "run read-only Linux route discovery")
	requirePKExec := fs.Bool("require-pkexec", false, "check that pkexec is available when not running as root")
	options := planConnectFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: big-red-button linux-preflight [-json] [-discover-routes] [-require-pkexec] [-endpoint-ip ip[,ip]] <profile.json>")
		return 2
	}
	if currentGOOS != "linux" {
		fmt.Fprintf(stderr, "linux-preflight can only run on Linux; current OS is %s\n", currentGOOS)
		return 1
	}

	output := linuxPreflightOutput{}
	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		output.Checks = append(output.Checks, preflightCheck{
			Name:   "profile",
			Status: "failed",
			Detail: err.Error(),
		})
		if *jsonOutput {
			writeJSON(stdout, output)
		} else {
			printLinuxPreflight(output, stdout)
		}
		return 1
	}
	summary := config.Summary()
	output.Profile = &summary
	output.Checks = append(output.Checks, preflightCheck{Name: "profile", Status: "ok", Detail: "profile is valid"})

	endpointIPs := csvOption(*options.endpointIPs)
	if len(endpointIPs) == 0 {
		endpointIPs, err = resolveEndpointIPs(context.Background(), config.WSTunnelHost)
		if err != nil {
			output.Checks = append(output.Checks, preflightCheck{Name: "resolve endpoint", Status: "failed", Detail: err.Error()})
			if *jsonOutput {
				writeJSON(stdout, output)
			} else {
				printLinuxPreflight(output, stdout)
			}
			return 1
		}
	}
	output.EndpointIPs = endpointIPs
	output.Checks = append(output.Checks, preflightCheck{Name: "resolve endpoint", Status: "ok", Detail: strings.Join(endpointIPs, ", ")})

	plan, err := buildConnectPlan(config, options, endpointIPs)
	if err != nil {
		output.Checks = append(output.Checks, preflightCheck{Name: "plan", Status: "failed", Detail: err.Error()})
		if *jsonOutput {
			writeJSON(stdout, output)
		} else {
			printLinuxPreflight(output, stdout)
		}
		return 1
	}
	output.Plan = &plan
	output.Checks = append(output.Checks, preflightCheck{Name: "plan", Status: "ok", Detail: "connect plan is buildable"})
	output.Checks = append(output.Checks, linuxPrerequisiteChecks(plan)...)
	if *requirePKExec {
		output.Checks = append(output.Checks, linuxPKExecCheck())
	}
	if *discoverRoutes {
		output.Checks = append(output.Checks, linuxRouteDiscoveryChecks(context.Background(), endpointIPs)...)
	}

	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printLinuxPreflight(output, stdout)
	}
	if linuxPreflightOK(output) {
		return 0
	}
	return 1
}

func linuxPreflightIsolatedApp(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-preflight-isolated-app", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	requirePKExec := fs.Bool("require-pkexec", false, "check that pkexec is available when not running as root")
	options := isolatedAppFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(stderr, "usage: big-red-button linux-preflight-isolated-app [-json] [-require-pkexec] [-session-id uuid] [-app-id uuid] [-dns ip[,ip]] [-app-uid uid -app-gid gid] [-app-env KEY=value] <profile.json> -- <command> [args...]")
		return 2
	}
	if currentGOOS != "linux" {
		fmt.Fprintf(stderr, "linux-preflight-isolated-app can only run on Linux; current OS is %s\n", currentGOOS)
		return 1
	}

	output := linuxPreflightOutput{}
	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		output.Checks = append(output.Checks, preflightCheck{Name: "profile", Status: "failed", Detail: err.Error()})
		if *jsonOutput {
			writeJSON(stdout, output)
		} else {
			printLinuxPreflight(output, stdout)
		}
		return 1
	}
	summary := config.Summary()
	output.Profile = &summary
	output.Checks = append(output.Checks, preflightCheck{Name: "profile", Status: "ok", Detail: "profile is valid"})

	isolatedOptions, err := withDefaultSessionID(isolatedAppOptionsFromFlags(options, isolatedAppCommandArgs(fs.Args())))
	if err != nil {
		output.Checks = append(output.Checks, preflightCheck{Name: "session", Status: "failed", Detail: err.Error()})
		if *jsonOutput {
			writeJSON(stdout, output)
		} else {
			printLinuxPreflight(output, stdout)
		}
		return 1
	}
	isolatedOptions = withDefaultLaunchIdentity(isolatedOptions)
	isolatedOptions = withDefaultDesktopEnv(isolatedOptions)
	plan, err := planner.IsolatedAppTunnel(config, isolatedOptions)
	if err != nil {
		output.Checks = append(output.Checks, preflightCheck{Name: "plan", Status: "failed", Detail: err.Error()})
		if *jsonOutput {
			writeJSON(stdout, output)
		} else {
			printLinuxPreflight(output, stdout)
		}
		return 1
	}
	output.Plan = &plan
	output.Checks = append(output.Checks, preflightCheck{Name: "plan", Status: "ok", Detail: "isolated app plan is buildable"})
	output.Checks = append(output.Checks, linuxPrerequisiteChecks(plan)...)
	if *requirePKExec {
		output.Checks = append(output.Checks, linuxPKExecCheck())
	}
	output.Checks = append(output.Checks, linuxIsolatedAppExecutableCheck(plan))
	output.Checks = append(output.Checks, linuxIsolatedRuntimeCheck(plan))

	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printLinuxPreflight(output, stdout)
	}
	if linuxPreflightOK(output) {
		return 0
	}
	return 1
}

func linuxDryRunIsolatedApp(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-dry-run-isolated-app", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	options := isolatedAppFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(stderr, "usage: big-red-button linux-dry-run-isolated-app [-json] [-session-id uuid] [-app-id uuid] [-dns ip[,ip]] [-app-uid uid -app-gid gid] [-app-env KEY=value] <profile.json> -- <command> [args...]")
		return 2
	}

	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		printProfileError(err, stderr, *jsonOutput, stdout)
		return 1
	}
	isolatedOptions, err := withDefaultSessionID(isolatedAppOptionsFromFlags(options, isolatedAppCommandArgs(fs.Args())))
	if err != nil {
		fmt.Fprintf(stderr, "generate isolated session ID: %v\n", err)
		return 1
	}
	plan, err := planner.IsolatedAppTunnel(config, isolatedOptions)
	if err != nil {
		fmt.Fprintf(stderr, "build isolated app plan: %v\n", err)
		return 1
	}
	executor, err := platformlinux.NewDryRunExecutorWithOptions(plan, platformlinux.DryRunOptions{
		RuntimeRoot: plan.RuntimeRoot,
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
		fmt.Fprintln(stderr, "usage: big-red-button linux-dry-run-disconnect [-json] [-persist-runtime-state] [-wireguard-interface name] [-runtime-root path]")
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
	if output.Result.State != engine.StateIdle {
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
		fmt.Fprintln(stderr, "usage: big-red-button linux-connect -yes [-json] [-endpoint-ip ip[,ip]] <profile.json>")
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
	plan, err := buildConnectPlan(config, options, csvOption(*options.endpointIPs))
	if err != nil {
		fmt.Fprintf(stderr, "build connect plan: %v\n", err)
		return 1
	}
	if handled, code := handleExistingLinuxConnection(plan, *jsonOutput, stdout); handled {
		return code
	}
	if len(plan.EndpointIPs) == 0 {
		endpointIPs, err := resolveEndpointIPs(context.Background(), config.WSTunnelHost)
		if err != nil {
			fmt.Fprintf(stderr, "resolve WSTunnel endpoint: %v\n", err)
			return 1
		}
		plan, err = buildConnectPlan(config, options, endpointIPs)
		if err != nil {
			fmt.Fprintf(stderr, "build connect plan: %v\n", err)
			return 1
		}
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
		DNSOperations:       executor.DNSOperations(),
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

func handleExistingLinuxConnection(plan planner.Plan, jsonOutput bool, stdout io.Writer) (bool, int) {
	snapshot := status.FromStore(context.Background(), truntime.Store{Root: plan.RuntimeRoot})
	if snapshot.State == status.StateIdle {
		return false, 0
	}

	result := engine.Result{
		PlanKind: plan.Kind,
		State:    engine.StateConnected,
	}
	code := 0
	switch {
	case snapshot.State == status.StateDirty:
		result.State = engine.StateFailedDirty
		result.Error = "runtime state is dirty; run linux-disconnect or cleanup before connecting"
		code = 1
	case snapshot.Active == nil:
		result.State = engine.StateFailedDirty
		result.Error = "runtime state is active but missing details"
		code = 1
	case snapshot.Active.ProfileFingerprint != plan.ProfileFingerprint:
		result.State = engine.StateFailedDirty
		result.Error = "already connected with a different profile; disconnect before connecting"
		code = 1
	case snapshot.Active.WireGuardInterface != plan.WireGuardInterface:
		result.State = engine.StateFailedDirty
		result.Error = "already connected with a different WireGuard interface; disconnect before connecting"
		code = 1
	default:
		result.AppliedStepIDs = []string{"already-connected"}
	}

	output := linuxLifecycleOutput{
		Plan:   plan,
		Result: result,
	}
	if jsonOutput {
		writeJSON(stdout, output)
	} else {
		printPlan(plan, false, stdout)
		printLinuxLifecycle(output, stdout)
	}
	return true, code
}

func resolveEndpointIPs(ctx context.Context, host string) ([]string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	addrs, err := lookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var out []string
	for _, addr := range addrs {
		ip := strings.TrimSpace(addr.IP.String())
		if ip == "" || ip == "<nil>" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("host %s did not resolve to an IP address", host)
	}
	sort.Strings(out)
	return out, nil
}

func linuxIsolatedApp(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-isolated-app", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	confirmed := fs.Bool("yes", false, "confirm this command may change Linux namespace, process and firewall state")
	cleanupOnExit := fs.Bool("cleanup-on-exit", true, "start a background monitor that cleans up this isolated session when the app exits")
	options := isolatedAppFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(stderr, "usage: big-red-button linux-isolated-app -yes [-json] [-cleanup-on-exit=false] [-session-id uuid] [-app-id uuid] [-dns ip[,ip]] [-app-uid uid -app-gid gid] [-app-env KEY=value] <profile.json> -- <command> [args...]")
		return 2
	}
	if !*confirmed {
		fmt.Fprintln(stderr, "linux-isolated-app requires -yes because it changes Linux namespace, process and firewall state")
		return 2
	}
	if currentGOOS != "linux" {
		fmt.Fprintf(stderr, "linux-isolated-app can only run on Linux; current OS is %s\n", currentGOOS)
		return 1
	}

	config, err := profile.LoadFile(fs.Arg(0))
	if err != nil {
		printProfileError(err, stderr, *jsonOutput, stdout)
		return 1
	}
	isolatedOptions, err := withDefaultSessionID(isolatedAppOptionsFromFlags(options, isolatedAppCommandArgs(fs.Args())))
	if err != nil {
		fmt.Fprintf(stderr, "generate isolated session ID: %v\n", err)
		return 1
	}
	isolatedOptions = withDefaultLaunchIdentity(isolatedOptions)
	isolatedOptions = withDefaultDesktopEnv(isolatedOptions)
	plan, err := planner.IsolatedAppTunnel(config, isolatedOptions)
	if err != nil {
		fmt.Fprintf(stderr, "build isolated app plan: %v\n", err)
		return 1
	}
	executor, err := platformlinux.NewIsolatedExecutor(platformlinux.IsolatedExecutorOptions{
		Plan:        plan,
		Profile:     config,
		RuntimeRoot: plan.RuntimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build linux isolated executor: %v\n", err)
		return 1
	}
	result := engine.New(executor).Run(context.Background(), plan)
	output := linuxIsolatedOutput{
		Plan:       plan,
		Result:     result,
		Operations: executor.Operations(),
	}
	if result.State == engine.StateConnected && *cleanupOnExit {
		monitorPID, err := startLinuxIsolatedMonitor(plan.SessionID, plan.RuntimeRoot)
		if err != nil {
			output.MonitorError = err.Error()
		} else {
			output.MonitorStarted = true
			output.MonitorPID = monitorPID
		}
	}
	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printPlan(plan, false, stdout)
		printLinuxIsolated(output, stdout)
	}
	if result.State != engine.StateConnected {
		return 1
	}
	if *cleanupOnExit && output.MonitorError != "" {
		return 1
	}
	return 0
}

func linuxMonitorIsolatedApp(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-monitor-isolated-app", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	confirmed := fs.Bool("yes", false, "confirm this command may wait for and clean up Linux isolated session state")
	options := isolatedStopFlags(fs)
	pollInterval := fs.Duration("poll-interval", 2*time.Second, "process polling interval")
	waitTimeout := fs.Duration("wait-timeout", 0, "maximum wait before aborting; 0 waits indefinitely")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: big-red-button linux-monitor-isolated-app -yes [-json] -session-id uuid [-runtime-root path] [-poll-interval duration] [-wait-timeout duration]")
		return 2
	}
	if !*confirmed {
		fmt.Fprintln(stderr, "linux-monitor-isolated-app requires -yes because it may remove launcher-owned Linux namespace/firewall/runtime state")
		return 2
	}
	if currentGOOS != "linux" {
		fmt.Fprintf(stderr, "linux-monitor-isolated-app can only run on Linux; current OS is %s\n", currentGOOS)
		return 1
	}
	if *pollInterval <= 0 {
		fmt.Fprintln(stderr, "poll interval must be positive")
		return 2
	}

	runtimeRoot := strings.TrimSpace(*options.runtimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = planner.DefaultRuntimeRoot
	}
	sessionID := strings.TrimSpace(*options.sessionID)
	if sessionID == "" {
		fmt.Fprintln(stderr, "session ID is required")
		return 2
	}
	store := truntime.Store{Root: filepath.Join(runtimeRoot, planner.DefaultIsolatedRuntimeSubdir, sessionID)}
	if err := recordCurrentMonitorProcess(context.Background(), store); err != nil {
		fmt.Fprintf(stderr, "record isolated monitor process: %v\n", err)
		return 1
	}
	waitStarted := time.Now()
	monitor, err := waitForIsolatedAppExit(context.Background(), store, *pollInterval, *waitTimeout)
	if err != nil {
		fmt.Fprintf(stderr, "monitor isolated app: %v\n", err)
		return 1
	}
	monitor.WaitDuration = time.Since(waitStarted).Round(time.Millisecond).String()
	output := linuxIsolatedMonitorOutput{
		SessionID:   sessionID,
		RuntimeRoot: runtimeRoot,
		Monitor:     monitor,
	}

	plan, err := planner.IsolatedAppStop(planner.IsolatedAppStopOptions{
		SessionID:   sessionID,
		RuntimeRoot: runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build isolated monitor cleanup plan: %v\n", err)
		return 1
	}
	cleanup, err := runLinuxIsolatedPlan(plan)
	if err != nil {
		fmt.Fprintf(stderr, "run isolated monitor cleanup: %v\n", err)
		return 1
	}
	output.Cleanup = &cleanup
	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printLinuxIsolatedMonitor(output, stdout)
	}
	if cleanup.Result.State != engine.StateIdle {
		return 1
	}
	return 0
}

func linuxStopIsolatedApp(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-stop-isolated-app", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	confirmed := fs.Bool("yes", false, "confirm this command may stop processes and remove Linux namespace/firewall state")
	options := isolatedStopFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: big-red-button linux-stop-isolated-app -yes [-json] -session-id uuid [-runtime-root path]")
		return 2
	}
	if !*confirmed {
		fmt.Fprintln(stderr, "linux-stop-isolated-app requires -yes because it stops processes and removes Linux namespace/firewall state")
		return 2
	}
	if currentGOOS != "linux" {
		fmt.Fprintf(stderr, "linux-stop-isolated-app can only run on Linux; current OS is %s\n", currentGOOS)
		return 1
	}

	plan, err := planner.IsolatedAppStop(planner.IsolatedAppStopOptions{
		SessionID:   *options.sessionID,
		RuntimeRoot: *options.runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build isolated stop plan: %v\n", err)
		return 1
	}
	executor, err := platformlinux.NewIsolatedExecutor(platformlinux.IsolatedExecutorOptions{
		Plan:        plan,
		RuntimeRoot: plan.RuntimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build linux isolated executor: %v\n", err)
		return 1
	}
	result := engine.New(executor).Run(context.Background(), plan)
	output := linuxIsolatedOutput{
		Plan:       plan,
		Result:     result,
		Operations: executor.Operations(),
	}
	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printPlan(plan, false, stdout)
		printLinuxIsolated(output, stdout)
	}
	if output.Result.State != engine.StateIdle {
		return 1
	}
	return 0
}

func linuxCleanupIsolatedApp(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-cleanup-isolated-app", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	confirmed := fs.Bool("yes", false, "confirm this command may remove launcher-owned Linux namespace/firewall/runtime state")
	options := isolatedStopFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: big-red-button linux-cleanup-isolated-app -yes [-json] -session-id uuid [-runtime-root path]")
		return 2
	}
	if !*confirmed {
		fmt.Fprintln(stderr, "linux-cleanup-isolated-app requires -yes because it removes launcher-owned Linux namespace/firewall/runtime state")
		return 2
	}
	if currentGOOS != "linux" {
		fmt.Fprintf(stderr, "linux-cleanup-isolated-app can only run on Linux; current OS is %s\n", currentGOOS)
		return 1
	}

	plan, err := planner.IsolatedAppCleanup(planner.IsolatedAppStopOptions{
		SessionID:   *options.sessionID,
		RuntimeRoot: *options.runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build isolated cleanup plan: %v\n", err)
		return 1
	}
	output, err := runLinuxIsolatedPlan(plan)
	if err != nil {
		fmt.Fprintf(stderr, "run linux isolated cleanup: %v\n", err)
		return 1
	}
	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printPlan(plan, false, stdout)
		printLinuxIsolated(output, stdout)
	}
	if output.Result.State != engine.StateIdle {
		return 1
	}
	return 0
}

func linuxRecoverIsolatedSessions(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("linux-recover-isolated-sessions", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	confirmed := fs.Bool("yes", false, "confirm this command may remove launcher-owned Linux namespace/firewall/runtime state")
	includeAll := fs.Bool("all", false, "recover all known isolated sessions, including sessions that still look connected")
	runtimeRoot := fs.String("runtime-root", planner.DefaultRuntimeRoot, "launcher runtime state root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: big-red-button linux-recover-isolated-sessions -yes [-json] [-all] [-runtime-root path]")
		return 2
	}
	if !*confirmed {
		fmt.Fprintln(stderr, "linux-recover-isolated-sessions requires -yes because it removes launcher-owned Linux namespace/firewall/runtime state")
		return 2
	}
	if currentGOOS != "linux" {
		fmt.Fprintf(stderr, "linux-recover-isolated-sessions can only run on Linux; current OS is %s\n", currentGOOS)
		return 1
	}

	sessions, err := status.IsolatedSessions(context.Background(), *runtimeRoot)
	if err != nil {
		fmt.Fprintf(stderr, "list isolated sessions: %v\n", err)
		return 1
	}
	targets, skipped := isolatedRecoveryTargets(sessions, *includeAll)
	output := linuxIsolatedRecoveryOutput{
		RuntimeRoot: *runtimeRoot,
		All:         *includeAll,
		Targets:     targets,
		Skipped:     skipped,
	}
	code := 0
	for _, sessionID := range targets {
		plan, err := planner.IsolatedAppCleanup(planner.IsolatedAppStopOptions{
			SessionID:   sessionID,
			RuntimeRoot: *runtimeRoot,
		})
		if err != nil {
			fmt.Fprintf(stderr, "build isolated cleanup plan for %s: %v\n", sessionID, err)
			code = 1
			continue
		}
		sessionOutput, err := runLinuxIsolatedPlan(plan)
		if err != nil {
			fmt.Fprintf(stderr, "run isolated cleanup for %s: %v\n", sessionID, err)
			code = 1
			continue
		}
		if sessionOutput.Result.State != engine.StateIdle {
			code = 1
		}
		output.Sessions = append(output.Sessions, sessionOutput)
	}

	if *jsonOutput {
		writeJSON(stdout, output)
	} else {
		printLinuxIsolatedRecovery(output, stdout)
	}
	return code
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
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "usage: big-red-button linux-disconnect -yes [-json] [-runtime-root path] [profile.json]")
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

	var config profile.Config
	if fs.NArg() == 1 {
		var err error
		config, err = profile.LoadFile(fs.Arg(0))
		if err != nil {
			printProfileError(err, stderr, *jsonOutput, stdout)
			return 1
		}
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
		DNSOperations:       executor.DNSOperations(),
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
		fmt.Fprintln(stderr, "usage: big-red-button plan-disconnect [-json]")
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

func buildConnectPlan(config profile.Config, options connectFlagValues, endpointIPs []string) (planner.Plan, error) {
	return planner.Connect(config, planner.Options{
		EndpointIPs:        endpointIPs,
		DefaultGateway:     *options.defaultGateway,
		DefaultInterface:   *options.defaultInterface,
		WSTunnelBinary:     *options.wstunnelBinary,
		WireGuardInterface: *options.wireguardInterface,
		RuntimeRoot:        *options.runtimeRoot,
	})
}

type connectFlagValues struct {
	endpointIPs        *string
	defaultGateway     *string
	defaultInterface   *string
	wstunnelBinary     *string
	wireguardInterface *string
	runtimeRoot        *string
}

type isolatedAppFlagValues struct {
	sessionID          *string
	appID              *string
	dns                *string
	wstunnelBinary     *string
	wireguardInterface *string
	runtimeRoot        *string
	namespace          *string
	hostVeth           *string
	namespaceVeth      *string
	hostAddress        *string
	namespaceAddress   *string
	hostGateway        *string
	launchUID          *string
	launchGID          *string
	launchEnv          *stringListFlag
}

type isolatedStopFlagValues struct {
	sessionID   *string
	runtimeRoot *string
}

type linuxDryRunOutput struct {
	Plan       planner.Plan              `json:"plan"`
	Result     engine.Result             `json:"result"`
	Operations []platformlinux.Operation `json:"operations"`
}

type linuxPreflightOutput struct {
	Profile     *profile.Summary `json:"profile,omitempty"`
	EndpointIPs []string         `json:"endpoint_ips,omitempty"`
	Plan        *planner.Plan    `json:"plan,omitempty"`
	Checks      []preflightCheck `json:"checks"`
}

type preflightCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type diagnosticsHost struct {
	Executable   string           `json:"executable,omitempty"`
	EffectiveUID int              `json:"effective_uid"`
	Checks       []preflightCheck `json:"checks,omitempty"`
}

type linuxLifecycleOutput struct {
	Plan                planner.Plan                   `json:"plan"`
	Result              engine.Result                  `json:"result"`
	RouteOperations     []platformlinux.Operation      `json:"route_operations,omitempty"`
	DNSOperations       []platformlinux.Operation      `json:"dns_operations,omitempty"`
	WSTunnelOperations  []supervisor.WSTunnelOperation `json:"wstunnel_operations,omitempty"`
	WireGuardOperations []platformlinux.Operation      `json:"wireguard_operations,omitempty"`
}

type linuxIsolatedOutput struct {
	Plan           planner.Plan              `json:"plan"`
	Result         engine.Result             `json:"result"`
	Operations     []platformlinux.Operation `json:"operations,omitempty"`
	MonitorStarted bool                      `json:"monitor_started,omitempty"`
	MonitorPID     int                       `json:"monitor_pid,omitempty"`
	MonitorError   string                    `json:"monitor_error,omitempty"`
}

type linuxIsolatedMonitorState struct {
	AppPID       int    `json:"app_pid,omitempty"`
	Reason       string `json:"reason"`
	WaitDuration string `json:"wait_duration,omitempty"`
}

type linuxIsolatedMonitorOutput struct {
	SessionID   string                    `json:"session_id"`
	RuntimeRoot string                    `json:"runtime_root"`
	Monitor     linuxIsolatedMonitorState `json:"monitor"`
	Cleanup     *linuxIsolatedOutput      `json:"cleanup,omitempty"`
}

type linuxIsolatedRecoveryOutput struct {
	RuntimeRoot string                `json:"runtime_root"`
	All         bool                  `json:"all"`
	Targets     []string              `json:"targets"`
	Sessions    []linuxIsolatedOutput `json:"sessions,omitempty"`
	Skipped     []string              `json:"skipped,omitempty"`
}

type diagnosticsOutput struct {
	GeneratedAt      string                           `json:"generated_at"`
	Version          buildinfo.Info                   `json:"version"`
	OS               string                           `json:"os"`
	Host             diagnosticsHost                  `json:"host"`
	RuntimeRoot      string                           `json:"runtime_root"`
	Runtime          status.Snapshot                  `json:"runtime"`
	IsolatedSessions []status.IsolatedSessionSnapshot `json:"isolated_sessions,omitempty"`
	ProfilePath      string                           `json:"profile_path,omitempty"`
	Profile          *profile.Summary                 `json:"profile,omitempty"`
	ProfileError     string                           `json:"profile_error,omitempty"`
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
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

func isolatedAppFlags(fs *flag.FlagSet) isolatedAppFlagValues {
	launchEnv := stringListFlag{}
	fs.Var(&launchEnv, "app-env", "safe desktop environment KEY=value for the isolated app; repeatable")
	return isolatedAppFlagValues{
		sessionID:          fs.String("session-id", "", "isolated session UUID"),
		appID:              fs.String("app-id", "", "app profile UUID; defaults to session-id"),
		dns:                fs.String("dns", "", "comma-separated namespace DNS servers; defaults to profile DNS"),
		wstunnelBinary:     fs.String("wstunnel-binary", "", "WSTunnel binary path/name"),
		wireguardInterface: fs.String("wireguard-interface", "", "WireGuard interface name inside namespace"),
		runtimeRoot:        fs.String("runtime-root", "", "launcher runtime state root"),
		namespace:          fs.String("namespace", "", "Linux network namespace name"),
		hostVeth:           fs.String("host-veth", "", "host-side veth name"),
		namespaceVeth:      fs.String("namespace-veth", "", "namespace-side veth name"),
		hostAddress:        fs.String("host-address", "", "host-side veth CIDR"),
		namespaceAddress:   fs.String("namespace-address", "", "namespace-side veth CIDR"),
		hostGateway:        fs.String("host-gateway", "", "host-side veth gateway address"),
		launchUID:          fs.String("app-uid", "", "UID used to launch the selected app inside the namespace"),
		launchGID:          fs.String("app-gid", "", "GID used to launch the selected app inside the namespace"),
		launchEnv:          &launchEnv,
	}
}

func isolatedStopFlags(fs *flag.FlagSet) isolatedStopFlagValues {
	return isolatedStopFlagValues{
		sessionID:   fs.String("session-id", "", "isolated session UUID"),
		runtimeRoot: fs.String("runtime-root", "", "launcher runtime state root"),
	}
}

func isolatedAppOptionsFromFlags(flags isolatedAppFlagValues, appCommand []string) planner.IsolatedAppOptions {
	return planner.IsolatedAppOptions{
		SessionID:          *flags.sessionID,
		AppID:              *flags.appID,
		AppCommand:         appCommand,
		DNS:                csvOption(*flags.dns),
		WSTunnelBinary:     *flags.wstunnelBinary,
		WireGuardInterface: *flags.wireguardInterface,
		RuntimeRoot:        *flags.runtimeRoot,
		Namespace:          *flags.namespace,
		HostVeth:           *flags.hostVeth,
		NamespaceVeth:      *flags.namespaceVeth,
		HostAddress:        *flags.hostAddress,
		NamespaceAddress:   *flags.namespaceAddress,
		HostGateway:        *flags.hostGateway,
		LaunchUID:          *flags.launchUID,
		LaunchGID:          *flags.launchGID,
		LaunchEnv:          append([]string(nil), (*flags.launchEnv)...),
	}
}

func withDefaultSessionID(options planner.IsolatedAppOptions) (planner.IsolatedAppOptions, error) {
	if strings.TrimSpace(options.SessionID) != "" {
		return options, nil
	}
	sessionID, err := randomUUID()
	if err != nil {
		return options, err
	}
	options.SessionID = sessionID
	return options, nil
}

func randomUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func withDefaultLaunchIdentity(options planner.IsolatedAppOptions) planner.IsolatedAppOptions {
	if strings.TrimSpace(options.LaunchUID) != "" || strings.TrimSpace(options.LaunchGID) != "" {
		return options
	}
	uid := strings.TrimSpace(os.Getenv("SUDO_UID"))
	gid := strings.TrimSpace(os.Getenv("SUDO_GID"))
	if uid == "" {
		uid = strings.TrimSpace(os.Getenv("PKEXEC_UID"))
	}
	if uid != "" && gid == "" {
		if user, err := userpkg.LookupId(uid); err == nil {
			gid = user.Gid
		}
	}
	if uid != "" && gid != "" {
		options.LaunchUID = uid
		options.LaunchGID = gid
	}
	return options
}

func withDefaultDesktopEnv(options planner.IsolatedAppOptions) planner.IsolatedAppOptions {
	existing := map[string]struct{}{}
	for _, value := range options.LaunchEnv {
		key, _, ok := strings.Cut(value, "=")
		if ok {
			existing[key] = struct{}{}
		}
	}
	for _, key := range desktopEnvKeys() {
		if _, ok := existing[key]; ok {
			continue
		}
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			options.LaunchEnv = append(options.LaunchEnv, key+"="+value)
		}
	}
	return options
}

func desktopEnvKeys() []string {
	return []string{"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "PULSE_SERVER", "PIPEWIRE_RUNTIME_DIR"}
}

func isolatedAppCommandArgs(args []string) []string {
	if len(args) < 2 {
		return nil
	}
	if args[1] == "--" {
		return args[2:]
	}
	return args[1:]
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
	if plan.SessionID != "" {
		fmt.Fprintf(stdout, "session: %s\n", plan.SessionID)
	}
	if plan.AppID != "" {
		fmt.Fprintf(stdout, "app id: %s\n", plan.AppID)
	}
	if plan.Namespace != "" {
		fmt.Fprintf(stdout, "namespace: %s\n", plan.Namespace)
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

func printLinuxPreflight(output linuxPreflightOutput, stdout io.Writer) {
	if output.Plan != nil && output.Plan.Kind == planner.IsolatedAppTunnelKind {
		fmt.Fprintln(stdout, "linux isolated app preflight")
	} else {
		fmt.Fprintln(stdout, "linux preflight")
	}
	if output.Profile != nil {
		fmt.Fprintf(stdout, "profile fingerprint: %s\n", output.Profile.Fingerprint)
		fmt.Fprintf(stdout, "profile gateway: %s\n", output.Profile.WSTunnelURL)
	}
	if len(output.EndpointIPs) > 0 {
		fmt.Fprintf(stdout, "endpoint IPs: %v\n", output.EndpointIPs)
	}
	if output.Plan != nil && len(output.Plan.Warnings) > 0 {
		fmt.Fprintln(stdout, "warnings:")
		for _, warning := range output.Plan.Warnings {
			fmt.Fprintf(stdout, "- %s\n", warning)
		}
	}
	if len(output.Checks) == 0 {
		fmt.Fprintln(stdout, "checks: []")
		return
	}
	printChecks(stdout, output.Checks)
}

func linuxPreflightOK(output linuxPreflightOutput) bool {
	if len(output.Checks) == 0 {
		return false
	}
	for _, check := range output.Checks {
		if check.Status != "ok" {
			return false
		}
	}
	return true
}

func linuxPrerequisiteChecks(plan planner.Plan) []preflightCheck {
	var checks []preflightCheck
	for _, step := range plan.Steps {
		if step.ID != "validate-linux-prerequisites" {
			continue
		}
		for _, binary := range stepDetailValues(step, "binary") {
			checks = append(checks, executableCheck("binary "+binary, binary))
		}
	}
	return checks
}

func linuxPKExecCheck() preflightCheck {
	if currentEUID() == 0 {
		return preflightCheck{Name: "privilege helper pkexec", Status: "ok", Detail: "running as root; pkexec is not required"}
	}
	return executableCheck("privilege helper pkexec", "pkexec")
}

func executableCheck(name string, binary string) preflightCheck {
	check := preflightCheck{Name: name, Status: "ok", Detail: "found"}
	if err := platformlinux.ValidateExecutable(executableLookPath, binary); err != nil {
		check.Status = "failed"
		check.Detail = err.Error()
	}
	return check
}

func linuxIsolatedAppExecutableCheck(plan planner.Plan) preflightCheck {
	executable := ""
	for _, step := range plan.Steps {
		if step.ID == "validate-app-command" {
			values := stepDetailValues(step, "executable")
			if len(values) > 0 {
				executable = values[0]
			}
			break
		}
	}
	if executable == "" {
		return preflightCheck{Name: "app executable", Status: "failed", Detail: "app executable is missing from plan"}
	}
	return executableCheck("app "+executable, executable)
}

func linuxIsolatedRuntimeCheck(plan planner.Plan) preflightCheck {
	if strings.TrimSpace(plan.RuntimeRoot) == "" || strings.TrimSpace(plan.SessionID) == "" {
		return preflightCheck{Name: "isolated runtime", Status: "failed", Detail: "runtime root or session ID is missing from plan"}
	}
	snapshot := status.FromStore(context.Background(), truntime.Store{
		Root: filepath.Join(plan.RuntimeRoot, planner.DefaultIsolatedRuntimeSubdir, plan.SessionID),
	})
	if snapshot.State == status.StateIdle {
		return preflightCheck{Name: "isolated runtime", Status: "ok", Detail: "no active session"}
	}
	detail := string(snapshot.State)
	if snapshot.Error != "" {
		detail += ": " + snapshot.Error
	}
	return preflightCheck{Name: "isolated runtime", Status: "failed", Detail: detail}
}

func linuxRouteDiscoveryChecks(ctx context.Context, endpointIPs []string) []preflightCheck {
	checks := make([]preflightCheck, 0, len(endpointIPs))
	for _, endpointIP := range endpointIPs {
		check := preflightCheck{Name: "route " + endpointIP, Status: "ok"}
		exclusion, err := platformlinux.DiscoverEndpointExclusion(ctx, preflightCommandRunner, endpointIP)
		if err != nil {
			check.Status = "failed"
			check.Detail = err.Error()
		} else {
			check.Detail = routeExclusionDetail(exclusion)
		}
		checks = append(checks, check)
	}
	return checks
}

func routeExclusionDetail(exclusion routes.EndpointExclusion) string {
	var parts []string
	if exclusion.Destination != "" {
		parts = append(parts, exclusion.Destination)
	}
	if exclusion.Gateway != "" {
		parts = append(parts, "via "+exclusion.Gateway)
	}
	if exclusion.Interface != "" {
		parts = append(parts, "dev "+exclusion.Interface)
	}
	return strings.Join(parts, " ")
}

func stepDetailValues(step planner.Step, key string) []string {
	prefix := key + "="
	var values []string
	for _, detail := range step.Details {
		if value, ok := strings.CutPrefix(detail, prefix); ok {
			values = append(values, value)
		}
	}
	return values
}

func printLinuxLifecycle(output linuxLifecycleOutput, stdout io.Writer) {
	fmt.Fprintf(stdout, "engine state: %s\n", output.Result.State)
	if output.Result.Error != "" {
		fmt.Fprintf(stdout, "engine error: %s\n", output.Result.Error)
	}
	if output.Result.RollbackError != "" {
		fmt.Fprintf(stdout, "rollback error: %s\n", output.Result.RollbackError)
	}
	for _, stepID := range output.Result.AppliedStepIDs {
		if stepID == "already-connected" {
			fmt.Fprintln(stdout, "engine note: already connected; no changes applied")
		}
	}
	printLinuxOperations(stdout, "route operations", output.RouteOperations)
	printLinuxOperations(stdout, "dns operations", output.DNSOperations)
	printWSTunnelOperations(stdout, output.WSTunnelOperations)
	printLinuxOperations(stdout, "wireguard operations", output.WireGuardOperations)
}

func printLinuxIsolated(output linuxIsolatedOutput, stdout io.Writer) {
	fmt.Fprintf(stdout, "engine state: %s\n", output.Result.State)
	if output.Result.Error != "" {
		fmt.Fprintf(stdout, "engine error: %s\n", output.Result.Error)
	}
	if output.Result.RollbackError != "" {
		fmt.Fprintf(stdout, "rollback error: %s\n", output.Result.RollbackError)
	}
	if output.MonitorStarted {
		fmt.Fprintf(stdout, "monitor pid: %d\n", output.MonitorPID)
	}
	if output.MonitorError != "" {
		fmt.Fprintf(stdout, "monitor error: %s\n", output.MonitorError)
	}
	printLinuxOperations(stdout, "isolated operations", output.Operations)
}

func printLinuxIsolatedMonitor(output linuxIsolatedMonitorOutput, stdout io.Writer) {
	fmt.Fprintf(stdout, "isolated monitor session: %s\n", output.SessionID)
	fmt.Fprintf(stdout, "isolated monitor root: %s\n", output.RuntimeRoot)
	if output.Monitor.AppPID > 0 {
		fmt.Fprintf(stdout, "app pid: %d\n", output.Monitor.AppPID)
	}
	fmt.Fprintf(stdout, "monitor reason: %s\n", output.Monitor.Reason)
	if output.Monitor.WaitDuration != "" {
		fmt.Fprintf(stdout, "wait duration: %s\n", output.Monitor.WaitDuration)
	}
	if output.Cleanup != nil {
		printLinuxIsolated(*output.Cleanup, stdout)
	}
}

func printLinuxIsolatedRecovery(output linuxIsolatedRecoveryOutput, stdout io.Writer) {
	fmt.Fprintf(stdout, "isolated recovery root: %s\n", output.RuntimeRoot)
	if output.All {
		fmt.Fprintln(stdout, "mode: all known sessions")
	} else {
		fmt.Fprintln(stdout, "mode: dirty sessions only")
	}
	if len(output.Targets) == 0 {
		fmt.Fprintln(stdout, "targets: []")
	} else {
		fmt.Fprintf(stdout, "targets: %v\n", output.Targets)
	}
	if len(output.Skipped) > 0 {
		fmt.Fprintf(stdout, "skipped: %v\n", output.Skipped)
	}
	for _, session := range output.Sessions {
		fmt.Fprintf(stdout, "\nsession: %s\n", session.Plan.SessionID)
		printLinuxIsolated(session, stdout)
	}
}

func printDiagnosticsOutput(stdout io.Writer, output diagnosticsOutput) {
	fmt.Fprintf(stdout, "generated at: %s\n", output.GeneratedAt)
	fmt.Fprintf(stdout, "version: %s\n", output.Version.Version)
	if output.Version.Commit != "" {
		fmt.Fprintf(stdout, "commit: %s\n", output.Version.Commit)
	}
	if output.Version.Date != "" {
		fmt.Fprintf(stdout, "built: %s\n", output.Version.Date)
	}
	fmt.Fprintf(stdout, "os: %s\n", output.OS)
	fmt.Fprintln(stdout, "host:")
	if output.Host.Executable != "" {
		fmt.Fprintf(stdout, "executable: %s\n", output.Host.Executable)
	}
	fmt.Fprintf(stdout, "effective uid: %d\n", output.Host.EffectiveUID)
	printChecks(stdout, output.Host.Checks)
	fmt.Fprintf(stdout, "runtime root: %s\n", output.RuntimeRoot)
	fmt.Fprintln(stdout, "system runtime:")
	printStatusSnapshot(stdout, output.Runtime)
	if output.ProfilePath != "" {
		fmt.Fprintf(stdout, "profile path: %s\n", output.ProfilePath)
		if output.Profile != nil {
			fmt.Fprintf(stdout, "profile fingerprint: %s\n", output.Profile.Fingerprint)
			fmt.Fprintf(stdout, "profile server: %s:%d\n", output.Profile.Server, output.Profile.Port)
			fmt.Fprintf(stdout, "profile gateway: %s\n", output.Profile.WSTunnelURL)
			fmt.Fprintf(stdout, "profile addresses: %v\n", output.Profile.Addresses)
			fmt.Fprintf(stdout, "profile allowed IPs: %v\n", output.Profile.AllowedIPs)
			fmt.Fprintf(stdout, "profile DNS configured: %t\n", output.Profile.DNSConfigured)
		}
		if output.ProfileError != "" {
			fmt.Fprintf(stdout, "profile error: %s\n", output.ProfileError)
		}
	}
	printIsolatedSessionList(stdout, output.IsolatedSessions, output.RuntimeRoot)
}

func printChecks(stdout io.Writer, checks []preflightCheck) {
	if len(checks) == 0 {
		return
	}
	fmt.Fprintln(stdout, "checks:")
	for _, check := range checks {
		if check.Detail == "" {
			fmt.Fprintf(stdout, "- %s %s\n", check.Status, check.Name)
			continue
		}
		fmt.Fprintf(stdout, "- %s %s: %s\n", check.Status, check.Name, check.Detail)
	}
}

func writeDiagnosticsBundle(path string, output diagnosticsOutput) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("diagnostics bundle path is required")
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create diagnostics bundle directory: %w", err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()

	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)

	diagnosticsJSON, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("encode diagnostics json: %w", err)
	}
	diagnosticsJSON = append(diagnosticsJSON, '\n')

	var diagnosticsText strings.Builder
	printDiagnosticsOutput(&diagnosticsText, output)

	manifest := map[string]any{
		"generated_at": output.GeneratedAt,
		"version":      output.Version,
		"os":           output.OS,
		"runtime_root": output.RuntimeRoot,
		"profile_path": output.ProfilePath,
		"contents": []string{
			"README.txt",
			"diagnostics.txt",
			"diagnostics.json",
			"manifest.json",
		},
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode diagnostics manifest: %w", err)
	}
	manifestJSON = append(manifestJSON, '\n')

	readme := strings.Join([]string{
		"Big Red Button diagnostics bundle",
		"",
		"This archive is intended for troubleshooting launcher state.",
		"It contains redacted status, profile summary and runtime metadata.",
		"It must not contain WireGuard private keys or preshared keys.",
		"",
	}, "\n")

	for _, entry := range []struct {
		name string
		data []byte
	}{
		{name: "README.txt", data: []byte(readme)},
		{name: "diagnostics.txt", data: []byte(diagnosticsText.String())},
		{name: "diagnostics.json", data: diagnosticsJSON},
		{name: "manifest.json", data: manifestJSON},
	} {
		if err := writeTarEntry(tarWriter, entry.name, entry.data, output.GeneratedAt); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := tarWriter.Close(); err != nil {
		_ = file.Close()
		return fmt.Errorf("close diagnostics tar: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		_ = file.Close()
		return fmt.Errorf("close diagnostics gzip: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close diagnostics bundle: %w", err)
	}
	closed = true
	return nil
}

func writeTarEntry(writer *tar.Writer, name string, data []byte, generatedAt string) error {
	modTime := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, generatedAt); err == nil {
		modTime = parsed
	}
	header := &tar.Header{
		Name:    name,
		Mode:    0o600,
		Size:    int64(len(data)),
		ModTime: modTime,
	}
	if err := writer.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header %s: %w", name, err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write tar entry %s: %w", name, err)
	}
	return nil
}

func defaultDiagnosticsBundlePath(generatedAt string) string {
	stamp := generatedAt
	if parsed, err := time.Parse(time.RFC3339, generatedAt); err == nil {
		stamp = parsed.UTC().Format("20060102-150405")
	} else {
		stamp = strings.NewReplacer(":", "", "-", "", "T", "-", "Z", "").Replace(generatedAt)
	}
	if stamp == "" {
		stamp = time.Now().UTC().Format("20060102-150405")
	}
	return "big-red-button-diagnostics-" + stamp + ".tar.gz"
}

func diagnosticsExitCode(output diagnosticsOutput) int {
	if output.Runtime.Error != "" || output.ProfileError != "" {
		return 1
	}
	for _, session := range output.IsolatedSessions {
		if session.Snapshot.Error != "" {
			return 1
		}
	}
	return 0
}

func printStatusSnapshot(stdout io.Writer, snapshot status.Snapshot) {
	fmt.Fprintf(stdout, "state: %s\n", snapshot.State)
	fmt.Fprintf(stdout, "runtime root: %s\n", snapshot.RuntimeRoot)
	if snapshot.Active != nil {
		if snapshot.Active.Mode != "" {
			fmt.Fprintf(stdout, "mode: %s\n", snapshot.Active.Mode)
		}
		if snapshot.Active.SessionID != "" {
			fmt.Fprintf(stdout, "session: %s\n", snapshot.Active.SessionID)
		}
		if snapshot.Active.Namespace != "" {
			fmt.Fprintf(stdout, "namespace: %s\n", snapshot.Active.Namespace)
		}
		fmt.Fprintf(stdout, "profile fingerprint: %s\n", snapshot.Active.ProfileFingerprint)
		fmt.Fprintf(stdout, "wireguard interface: %s\n", snapshot.Active.WireGuardInterface)
		if snapshot.Active.DNSApplied {
			fmt.Fprintf(stdout, "dns interface: %s\n", snapshot.Active.DNSInterface)
			fmt.Fprintf(stdout, "dns servers: %s\n", strings.Join(snapshot.Active.DNSServers, ", "))
		}
		if snapshot.Active.AppProcess != nil {
			fmt.Fprintf(stdout, "app pid: %d\n", snapshot.Active.AppProcess.PID)
		}
		if snapshot.Active.WSTunnelProcess != nil {
			fmt.Fprintf(stdout, "wstunnel pid: %d\n", snapshot.Active.WSTunnelProcess.PID)
		}
	}
	if snapshot.Error != "" {
		fmt.Fprintf(stdout, "error: %s\n", snapshot.Error)
	}
}

func printIsolatedSessionList(stdout io.Writer, sessions []status.IsolatedSessionSnapshot, runtimeRoot string) {
	if len(sessions) == 0 {
		fmt.Fprintln(stdout, "isolated sessions: []")
		return
	}
	runtimeRootArg := isolatedRuntimeRootArg(runtimeRoot)
	fmt.Fprintln(stdout, "isolated sessions:")
	for _, session := range sessions {
		fmt.Fprintf(stdout, "- session: %s\n", session.SessionID)
		fmt.Fprintf(stdout, "  state: %s\n", session.Snapshot.State)
		fmt.Fprintf(stdout, "  runtime root: %s\n", session.Snapshot.RuntimeRoot)
		if session.Snapshot.Active != nil {
			state := session.Snapshot.Active
			if state.Namespace != "" {
				fmt.Fprintf(stdout, "  namespace: %s\n", state.Namespace)
			}
			if state.WireGuardInterface != "" {
				fmt.Fprintf(stdout, "  wireguard interface: %s\n", state.WireGuardInterface)
			}
			if state.AppProcess != nil {
				fmt.Fprintf(stdout, "  app pid: %d\n", state.AppProcess.PID)
			}
			if state.WSTunnelProcess != nil {
				fmt.Fprintf(stdout, "  wstunnel pid: %d\n", state.WSTunnelProcess.PID)
			}
		}
		if session.Snapshot.Error != "" {
			fmt.Fprintf(stdout, "  error: %s\n", session.Snapshot.Error)
		}
		fmt.Fprintf(stdout, "  stop: big-red-button linux-stop-isolated-app -yes -session-id %s%s\n", session.SessionID, runtimeRootArg)
		fmt.Fprintf(stdout, "  cleanup: big-red-button linux-cleanup-isolated-app -yes -session-id %s%s\n", session.SessionID, runtimeRootArg)
	}
}

func isolatedRuntimeRootArg(runtimeRoot string) string {
	runtimeRoot = strings.TrimSpace(runtimeRoot)
	if runtimeRoot == "" || runtimeRoot == planner.DefaultRuntimeRoot {
		return ""
	}
	return " -runtime-root " + shellQuote(runtimeRoot)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("/._+-=:", r) {
			continue
		}
		return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
	}
	return value
}

func appendError(existing string, next string) string {
	if existing == "" {
		return next
	}
	if next == "" {
		return existing
	}
	return existing + "; " + next
}

func isolatedRecoveryTargets(sessions []status.IsolatedSessionSnapshot, includeAll bool) ([]string, []string) {
	var targets []string
	var skipped []string
	for _, session := range sessions {
		switch {
		case includeAll:
			targets = append(targets, session.SessionID)
		case session.Snapshot.State == status.StateDirty:
			targets = append(targets, session.SessionID)
		default:
			skipped = append(skipped, session.SessionID)
		}
	}
	return targets, skipped
}

func startLinuxIsolatedMonitor(sessionID string, runtimeRoot string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("resolve current executable: %w", err)
	}
	args := []string{"linux-monitor-isolated-app", "-yes", "-session-id", sessionID}
	if strings.TrimSpace(runtimeRoot) != "" {
		args = append(args, "-runtime-root", runtimeRoot)
	}
	cmd := exec.Command(executable, args...)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start isolated monitor: %w", err)
	}
	return cmd.Process.Pid, nil
}

func waitForIsolatedAppExit(ctx context.Context, store truntime.Store, pollInterval time.Duration, waitTimeout time.Duration) (linuxIsolatedMonitorState, error) {
	if pollInterval <= 0 {
		return linuxIsolatedMonitorState{}, fmt.Errorf("poll interval must be positive")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if waitTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, waitTimeout)
		defer cancel()
	}
	for {
		state, err := store.Load(ctx)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return linuxIsolatedMonitorState{Reason: "runtime state missing"}, nil
			}
			return linuxIsolatedMonitorState{}, err
		}
		if state.Mode != planner.IsolatedAppTunnelKind {
			return linuxIsolatedMonitorState{}, fmt.Errorf("runtime state is not an isolated app tunnel session")
		}
		if state.AppProcess == nil {
			return linuxIsolatedMonitorState{Reason: "app process missing from runtime state"}, nil
		}
		if !monitorPIDExists(state.AppProcess.PID) {
			return linuxIsolatedMonitorState{
				AppPID: state.AppProcess.PID,
				Reason: "app process exited",
			}, nil
		}
		if err := sleepContext(ctx, pollInterval); err != nil {
			return linuxIsolatedMonitorState{AppPID: state.AppProcess.PID}, err
		}
	}
}

func recordCurrentMonitorProcess(ctx context.Context, store truntime.Store) error {
	state, err := store.Load(ctx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	state = state.WithMonitorProcess(os.Getpid(), os.Args)
	return store.Save(ctx, state)
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runLinuxIsolatedPlan(plan planner.Plan) (linuxIsolatedOutput, error) {
	executor, err := platformlinux.NewIsolatedExecutor(platformlinux.IsolatedExecutorOptions{
		Plan:        plan,
		RuntimeRoot: plan.RuntimeRoot,
	})
	if err != nil {
		return linuxIsolatedOutput{}, fmt.Errorf("build linux isolated executor: %w", err)
	}
	result := engine.New(executor).Run(context.Background(), plan)
	return linuxIsolatedOutput{
		Plan:       plan,
		Result:     result,
		Operations: executor.Operations(),
	}, nil
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
	fmt.Fprintln(w, "usage: big-red-button <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  version")
	fmt.Fprintln(w, "  validate-profile [-json] <profile.json>")
	fmt.Fprintln(w, "  plan-connect [-json] [-endpoint-ip ip[,ip]] <profile.json>")
	fmt.Fprintln(w, "  plan-isolated-app [-json] [-session-id uuid] [-app-id uuid] [-dns ip[,ip]] [-app-uid uid -app-gid gid] [-app-env KEY=value] <profile.json> -- <command> [args...]")
	fmt.Fprintln(w, "  plan-isolated-stop [-json] -session-id uuid [-runtime-root path]")
	fmt.Fprintln(w, "  plan-isolated-cleanup [-json] -session-id uuid [-runtime-root path]")
	fmt.Fprintln(w, "  plan-disconnect [-json]")
	fmt.Fprintln(w, "  status [-json] [-runtime-root path]")
	fmt.Fprintln(w, "  isolated-status [-json] -session-id uuid [-runtime-root path]")
	fmt.Fprintln(w, "  isolated-sessions [-json] [-runtime-root path]")
	fmt.Fprintln(w, "  diagnostics [-json] [-runtime-root path] [-profile profile.json] [-wstunnel-binary path]")
	fmt.Fprintln(w, "  diagnostics-bundle [-runtime-root path] [-profile profile.json] [-wstunnel-binary path] [-output path.tar.gz]")
	fmt.Fprintln(w, "  linux-dry-run-connect [-json] [-discover-routes] [-persist-runtime-state] [-endpoint-ip ip[,ip]] <profile.json>")
	fmt.Fprintln(w, "  linux-preflight [-json] [-discover-routes] [-require-pkexec] [-endpoint-ip ip[,ip]] <profile.json>")
	fmt.Fprintln(w, "  linux-preflight-isolated-app [-json] [-require-pkexec] [-session-id uuid] [-app-id uuid] [-dns ip[,ip]] [-app-uid uid -app-gid gid] [-app-env KEY=value] <profile.json> -- <command> [args...]")
	fmt.Fprintln(w, "  linux-dry-run-isolated-app [-json] [-session-id uuid] [-app-id uuid] [-dns ip[,ip]] [-app-uid uid -app-gid gid] [-app-env KEY=value] <profile.json> -- <command> [args...]")
	fmt.Fprintln(w, "  linux-dry-run-disconnect [-json] [-persist-runtime-state] [-wireguard-interface name] [-runtime-root path]")
	fmt.Fprintln(w, "  linux-connect -yes [-json] [-endpoint-ip ip[,ip]] <profile.json>")
	fmt.Fprintln(w, "  linux-isolated-app -yes [-json] [-cleanup-on-exit=false] [-session-id uuid] [-app-id uuid] [-dns ip[,ip]] [-app-uid uid -app-gid gid] [-app-env KEY=value] <profile.json> -- <command> [args...]")
	fmt.Fprintln(w, "  linux-monitor-isolated-app -yes [-json] -session-id uuid [-runtime-root path] [-poll-interval duration] [-wait-timeout duration]")
	fmt.Fprintln(w, "  linux-stop-isolated-app -yes [-json] -session-id uuid [-runtime-root path]")
	fmt.Fprintln(w, "  linux-cleanup-isolated-app -yes [-json] -session-id uuid [-runtime-root path]")
	fmt.Fprintln(w, "  linux-recover-isolated-sessions -yes [-json] [-all] [-runtime-root path]")
	fmt.Fprintln(w, "  linux-disconnect -yes [-json] [-runtime-root path] [profile.json]")
}
