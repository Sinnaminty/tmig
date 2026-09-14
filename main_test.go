package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMainProcess(t *testing.T) {
	// Re-execute the test binary to exercise main's real os.Exit behavior without
	// terminating the parent test runner or building a second executable.
	if os.Getenv("TMIG_TEST_PROCESS") == "1" {
		separator := slices.Index(os.Args, "--")
		os.Args = append([]string{"tmig"}, os.Args[separator+1:]...)
		main()
		os.Exit(0)
	}
	t.Parallel()
	db := filepath.Join(t.TempDir(), "tasks.db")
	for _, tt := range []struct {
		name        string
		args        []string
		code        int
		out, errOut string
	}{
		{"help", []string{"help"}, 0, "Usage:", ""},
		{"add", []string{"add", "Across processes"}, 0, "Added task 1.", ""},
		{"persisted list", []string{"list"}, 0, "Across processes", ""},
		{"invalid command", []string{"unknown"}, 1, "", "tmig: unknown command"},
		{"missing task", []string{"delete", "9999"}, 1, "", "task not found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"-test.run=^TestMainProcess$", "--", "--db", db}, tt.args...)
			cmd := exec.Command(os.Args[0], args...)
			cmd.Env = append(os.Environ(), "TMIG_TEST_PROCESS=1")
			var out, errOut bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errOut
			err := cmd.Run()
			code := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatal(err)
				}
				code = exitErr.ExitCode()
			}
			if code != tt.code {
				t.Fatalf("exit code = %d; want %d; stderr: %s", code, tt.code, errOut.String())
			}
			for name, pair := range map[string][2]string{"stdout": {out.String(), tt.out}, "stderr": {errOut.String(), tt.errOut}} {
				if (pair[1] == "" && pair[0] != "") || !strings.Contains(pair[0], pair[1]) {
					t.Errorf("%s = %q; want %q", name, pair[0], pair[1])
				}
			}
		})
	}
}
