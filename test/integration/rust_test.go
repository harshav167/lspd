//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestRustAnalyzerAvailable(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not installed")
	}
}
