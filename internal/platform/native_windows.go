package platform

import (
	"runtime"

	"golang.org/x/sys/windows"
)

// nativeArch asks IsWow64Process2, as an emulated process sees its own
// architecture everywhere else.
func nativeArch() string {
	// IsWow64Process2 is new in Windows 10 1709.
	if windows.NewLazySystemDLL("kernel32.dll").NewProc("IsWow64Process2").Find() == nil {
		var process, native uint16
		if windows.IsWow64Process2(windows.CurrentProcess(), &process, &native) == nil {
			if arch := machineArch(native); arch != "" {
				return arch
			}
		}
	}
	return runtime.GOARCH
}
