package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/MyHeartRaces/BigRedButton/internal/buildinfo"
	"github.com/MyHeartRaces/BigRedButton/internal/daemon"
	"github.com/MyHeartRaces/BigRedButton/internal/planner"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("big-red-buttond", flag.ContinueOnError)
	socketPath := fs.String("socket", daemon.DefaultSocketPath, "Unix domain socket path")
	socketModeValue := fs.String("socket-mode", "0600", "Unix socket file mode in octal")
	runtimeRoot := fs.String("runtime-root", planner.DefaultRuntimeRoot, "launcher runtime state root")
	cliPath := fs.String("cli", "", "big-red-button CLI path used by mutating lifecycle endpoints")
	showVersion := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Printf("Big Red Button daemon %s\n", buildinfo.DisplayVersion())
		return 0
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: big-red-buttond [-socket path] [-socket-mode octal] [-runtime-root path] [-cli path] [-version]")
		return 2
	}
	socketMode, err := strconv.ParseUint(*socketModeValue, 8, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid socket mode: %v\n", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	handler := daemon.NewHandler(daemon.Options{RuntimeRoot: *runtimeRoot, CLIPath: *cliPath})
	if err := daemon.ServeUnixWithMode(ctx, *socketPath, handler, os.FileMode(socketMode)); err != nil {
		fmt.Fprintf(os.Stderr, "big-red-buttond: %v\n", err)
		return 1
	}
	return 0
}
