package qtmeta

import "strings"

// TargetOS represents the operating system component of a target string.
type TargetOS string

const (
	Windows TargetOS = "windows"
	Linux   TargetOS = "linux"
	MacOS   TargetOS = "macos"
)

// TargetMeta holds metadata about a Qt target/architecture.
type TargetMeta struct {
	Name        string
	DisplayName string
	OS          TargetOS
}

var knownTargetMap = func() map[string]TargetMeta { //nolint:gochecknoglobals // package-level target lookup table
	targets := []TargetMeta{
		// Windows
		{Name: "win64_msvc2022_64", DisplayName: "MSVC 2022, 64-bit", OS: Windows},
		{Name: "win64_msvc2022_arm64", DisplayName: "MSVC 2022, ARM64", OS: Windows},
		{Name: "win64_msvc2019_64", DisplayName: "MSVC 2019, 64-bit", OS: Windows},
		{Name: "win64_mingw", DisplayName: "MinGW 13.1, 64-bit", OS: Windows},
		{Name: "win64_llvm_mingw", DisplayName: "LLVM/Clang MinGW, 64-bit", OS: Windows},
		// Linux
		{Name: "gcc_64", DisplayName: "GCC, 64-bit", OS: Linux},
		{Name: "linux_gcc_64", DisplayName: "GCC, 64-bit", OS: Linux},
		// macOS
		{Name: "macos", DisplayName: "macOS (universal)", OS: MacOS},
		{Name: "clang_64", DisplayName: "Clang, 64-bit", OS: MacOS},
	}
	m := make(map[string]TargetMeta, len(targets))
	for _, t := range targets {
		m[t.Name] = t
	}
	return m
}()

// LookupTarget returns metadata for a target string.
func LookupTarget(name string) (TargetMeta, bool) {
	t, ok := knownTargetMap[name]
	if !ok {
		// Make a best-effort display name from the name.
		t = TargetMeta{
			Name:        name,
			DisplayName: name,
			OS:          guessOS(name),
		}
	}
	return t, ok
}

func guessOS(name string) TargetOS {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "win"):
		return Windows
	case strings.Contains(lower, "linux") || strings.Contains(lower, "gcc"):
		return Linux
	case strings.Contains(lower, "mac") || strings.Contains(lower, "clang"):
		return MacOS
	default:
		return Linux
	}
}

// NormalizeTarget normalises target strings (e.g. trims whitespace, lowercases).
func NormalizeTarget(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}
