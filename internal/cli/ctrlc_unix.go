//go:build !windows

package cli

// ignoreCtrlC is a no-op on non-Windows platforms; ignoring SIGINT via
// [signal.Ignore] in execChild is sufficient there.
func ignoreCtrlC(bool) {}
