package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/MyHeartRaces/BigRedButton/internal/buildinfo"
	"github.com/MyHeartRaces/BigRedButton/internal/desktop"
)

func main() {
	addr := flag.String("addr", "", "listen address")
	noOpen := flag.Bool("no-open", false, "do not open the browser")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Fprintf(os.Stdout, "Big Red Button %s\n", buildinfo.DisplayVersion())
		return
	}
	if *noOpen && *addr == "" {
		*addr = "127.0.0.1:0"
	}

	if err := desktop.Run(context.Background(), desktop.Options{
		Addr:    *addr,
		OpenURL: !*noOpen,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
