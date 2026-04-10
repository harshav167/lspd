//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestPyrightAvailable(t *testing.T) {
	if _, err := exec.LookPath("pyright-langserver"); err != nil {
		t.Skip("pyright-langserver not installed")
	}
}
