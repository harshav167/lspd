//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestClangdAvailable(t *testing.T) {
	if _, err := exec.LookPath("clangd"); err != nil {
		t.Skip("clangd not installed")
	}
}
