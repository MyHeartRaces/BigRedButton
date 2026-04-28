package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

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
	runtimeRoot := fs.String("runtime-root", planner.DefaultRuntimeRoot, "launcher runtime state root")
	showVersion := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Printf("Big Red Button daemon %s\n", buildinfo.DisplayVersion())
		return 0
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: big-red-buttond [-socket path] [-runtime-root path] [-version]")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	handler := daemon.NewHandler(daemon.Options{RuntimeRoot: *runtimeRoot})
	if err := daemon.ServeUnix(ctx, *socketPath, handler); err != nil {
		fmt.Fprintf(os.Stderr, "big-red-buttond: %v\n", err)
		return 1
	}
	return 0
}
