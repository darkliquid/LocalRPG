// cmd/localrpg/gui_test.go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIGUICommandHelp(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "gui", "--help")
	out, err := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "Launch desktop GUI or browser app") && !strings.Contains(output, "Usage of gui") {
		t.Errorf("unexpected output: %s, err: %v", output, err)
	}
}
