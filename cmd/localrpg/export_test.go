// cmd/localrpg/export_test.go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIExportHelp(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "export", "--help")
	out, err := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "Usage: localrpg export") && !strings.Contains(output, "Usage of export") {
		t.Errorf("unexpected output: %s, err: %v", output, err)
	}
}
