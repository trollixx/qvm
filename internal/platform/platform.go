package platform

// Platform provides platform-specific information and defaults.
type Platform interface {
	// DefaultInstallDir returns the default Qt install directory.
	DefaultInstallDir() string
	// DefaultArch returns the recommended Qt arch for this machine.
	DefaultArch(qtVersion string) string
	// CheckCompilerPresent reports whether the expected compiler for arch is available.
	CheckCompilerPresent(arch string) (bool, string)
}
