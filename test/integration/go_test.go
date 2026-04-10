//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestGoplsAvailable(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
}
