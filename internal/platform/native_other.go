//go:build !windows

package platform

import "runtime"

func nativeArch() string {
	return runtime.GOARCH
}
