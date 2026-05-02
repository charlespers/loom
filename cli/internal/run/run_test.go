package run

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactDirCreated(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOOM_HOME", tmp)

	id, dir, err := allocateRun()
	if err != nil {
		t.Fatalf("allocateRun: %v", err)
	}
	if len(id) != 26 {
		t.Fatalf("expected 26-char ULID, got %q (len %d)", id, len(id))
	}
	if filepath.Dir(dir) != filepath.Join(tmp, "runs") {
		t.Fatalf("run dir not under LOOM_HOME/runs: %s", dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("run dir not created: %v", err)
	}
}

func TestRingPathDirCreated(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOOM_TMPDIR", tmp)

	id := "01J3KTV6S5ABCDEFGHJKMNPQRS"
	ring, err := ensureRingDir(id)
	if err != nil {
		t.Fatalf("ensureRingDir: %v", err)
	}
	if filepath.Dir(ring) != filepath.Join(tmp, "loom", id) {
		t.Fatalf("unexpected ring path %s", ring)
	}
	if _, err := os.Stat(filepath.Dir(ring)); err != nil {
		t.Fatalf("ring dir not created: %v", err)
	}
}
