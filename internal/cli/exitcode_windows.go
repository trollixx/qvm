package cli

import "os/exec"

// childExitCode maps a child's ExitError to our own exit code. Negative codes
// (process not exited normally) are clamped to 1: passing them to [os.Exit]
// is not portable.
func childExitCode(ee *exec.ExitError) int {
	if code := ee.ExitCode(); code >= 0 {
		return code
	}
	return 1
}
