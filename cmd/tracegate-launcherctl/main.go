package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tracegate/tracegate-launcher/internal/planner"
	"github.com/tracegate/tracegate-launcher/internal/profile"
)

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
	endpointIPs := fs.String("endpoint-ip", "", "comma-separated resolved WSTunnel endpoint IPs")
	defaultGateway := fs.String("default-gateway", "", "pre-tunnel default gateway for route exclusion")
	defaultInterface := fs.String("default-interface", "", "pre-tunnel default interface for route exclusion")
	wstunnelBinary := fs.String("wstunnel-binary", "", "WSTunnel binary path/name")
	wireguardInterface := fs.String("wireguard-interface", "", "WireGuard interface name")
	runtimeRoot := fs.String("runtime-root", "", "launcher runtime state root")
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
		EndpointIPs:        csvOption(*endpointIPs),
		DefaultGateway:     *defaultGateway,
		DefaultInterface:   *defaultInterface,
		WSTunnelBinary:     *wstunnelBinary,
		WireGuardInterface: *wireguardInterface,
		RuntimeRoot:        *runtimeRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build connect plan: %v\n", err)
		return 1
	}
	printPlan(plan, *jsonOutput, stdout)
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
}
