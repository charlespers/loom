// loom-daemon — M1 skeleton. In M1 the daemon is launched but does no
// real work; it parses --run-id and --ring-path, prints a banner on
// stderr, and waits for SIGTERM. M2 implements ring drain and artifact
// writing.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const Version = "0.1.0"

func main() {
	runID := flag.String("run-id", "", "run id (ULID)")
	ringPath := flag.String("ring-path", "", "path to mmap'd ring file")
	flag.Parse()

	if *runID == "" {
		fmt.Fprintln(os.Stderr, "loom-daemon: --run-id is required")
		os.Exit(2)
	}

	fmt.Fprintf(os.Stderr,
		"loom-daemon v%s · run-id=%s · ring=%s · waiting for SIGTERM\n",
		Version, *runID, *ringPath)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig

	fmt.Fprintln(os.Stderr, "loom-daemon: shutting down")
}
