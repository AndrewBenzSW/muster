package vcs

import (
	"fmt"
	"strings"
)

// mockCommandRunner creates a mock CommandRunner that returns predefined outputs.
type mockCommandRunner struct {
	outputs map[string]mockOutput // key: command string (name + args joined)
}

type mockOutput struct {
	stdout string
	stderr string
	err    error
}

func (m *mockCommandRunner) run(dir string, name string, args ...string) (string, string, error) {
	key := name + " " + joinArgs(args)
	output, ok := m.outputs[key]
	if !ok {
		return "", "", fmt.Errorf("unexpected command: %s", key)
	}
	return output.stdout, output.stderr, output.err
}

func joinArgs(args []string) string {
	return strings.Join(args, " ")
}
