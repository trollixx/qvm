package cli

import "syscall"

// ignoreCtrlC toggles this process's CTRL+C disposition via
// SetConsoleCtrlHandler(NULL, ignore). When set, Windows does not deliver
// CTRL_C_EVENT to this process at all, so Go's runtime handler never fires.
// Used while a child process is running: without it, Go's CTRL_C_EVENT
// dispatch (even with [signal.Ignore]) disturbs interactive children (e.g.
// nushell) when they Ctrl+C a grandchild, leaving stdin corrupted. Call it
// only after the child is spawned so the child does not inherit the
// disposition.
func ignoreCtrlC(ignore bool) {
	var add uintptr
	if ignore {
		add = 1
	}
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler")
	_, _, _ = proc.Call(0, add)
}
