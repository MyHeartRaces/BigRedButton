package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

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

func writeJSON(w io.Writer, payload any) {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: tracegate-launcherctl <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  validate-profile [-json] <profile.json>")
}
