package router

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRootFindsNearestMarker(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	project := filepath.Join(root, "project")
	nested := filepath.Join(project, "pkg", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	path := filepath.Join(nested, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if got := detectRoot(path, []string{"go.mod"}); got != project {
		t.Fatalf("expected root %q, got %q", project, got)
	}
}

func TestDetectRootFallsBackToFileDirWhenNoMarkerExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "pkg", "main.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if got := detectRoot(path, []string{"go.mod"}); got != filepath.Dir(path) {
		t.Fatalf("expected fallback dir %q, got %q", filepath.Dir(path), got)
	}
}
