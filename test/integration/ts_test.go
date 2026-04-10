//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestTypeScriptServerAvailable(t *testing.T) {
	if _, err := exec.LookPath("typescript-language-server"); err != nil {
		t.Skip("typescript-language-server not installed")
	}
}
