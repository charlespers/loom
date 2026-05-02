// Package run implements `loom run` — allocate a run-id, create the
// artifact directory, set env vars, exec the child. M1 does not launch
// the daemon; that arrives in M2.
package run

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"
)

// loomHome returns the artifact root, honoring $LOOM_HOME with a fallback
// to ~/.loom.
func loomHome() (string, error) {
	if h := os.Getenv("LOOM_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".loom"), nil
}

// loomTmpDir returns the ring-file root, honoring $LOOM_TMPDIR with a
// fallback to /tmp.
func loomTmpDir() string {
	if t := os.Getenv("LOOM_TMPDIR"); t != "" {
		return t
	}
	return "/tmp"
}

// allocateRun creates a fresh ULID-named run directory under LOOM_HOME/runs
// and returns the id and the directory path.
func allocateRun() (string, string, error) {
	home, err := loomHome()
	if err != nil {
		return "", "", err
	}
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
	dir := filepath.Join(home, "runs", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	return id, dir, nil
}

// ensureRingDir creates the per-run ring directory and returns the
// expected ring file path. The file itself is not created in M1; M2 will
// mmap it.
func ensureRingDir(runID string) (string, error) {
	dir := filepath.Join(loomTmpDir(), "loom", runID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "ring"), nil
}

// RunE is the body of `loom run`.
func RunE(cmd *cobra.Command, args []string) error {
	quiet, _ := cmd.Flags().GetBool("quiet")

	id, dir, err := allocateRun()
	if err != nil {
		return fmt.Errorf("allocate run dir: %w", err)
	}
	ring, err := ensureRingDir(id)
	if err != nil {
		return fmt.Errorf("ensure ring dir: %w", err)
	}

	if !quiet {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"loom · run %s · artifacts %s\n", id, dir)
	}

	child := exec.Command(args[0], args[1:]...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = append(os.Environ(),
		"LOOM_RUN_ID="+id,
		"LOOM_RING_PATH="+ring,
	)

	if err := child.Run(); err != nil {
		// Forward the exit code if the child exited.
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("exec child: %w", err)
	}
	return nil
}
