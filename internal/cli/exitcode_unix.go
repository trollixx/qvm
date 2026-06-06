//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// childExitCode maps a child's ExitError to our own exit code. Children
// terminated by a signal report ExitCode() == -1, which [os.Exit] would
// mangle (commonly to 255); follow the shell convention of 128+signal
// instead.
func childExitCode(ee *exec.ExitError) int {
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	if code := ee.ExitCode(); code >= 0 {
		return code
	}
	return 1
}
