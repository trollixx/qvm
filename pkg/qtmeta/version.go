package qtmeta

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a parsed Qt version number.
type Version struct {
	Major int
	Minor int
	Patch int
}

// ParseVersion parses a Qt version string like "6.10.0" or "5.15.18".
func ParseVersion(s string) (Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: expected major.minor.patch", s)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version in %q: %w", s, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version in %q: %w", s, err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version in %q: %w", s, err)
	}
	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

// String returns the version as "major.minor.patch".
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns negative, zero, or positive depending on whether v < other, v == other, or v > other.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return v.Major - other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor - other.Minor
	}
	return v.Patch - other.Patch
}

// Less reports whether v is older than other.
func (v Version) Less(other Version) bool {
	return v.Compare(other) < 0
}

// MajorVersion returns the major version number from a version string.
// Returns 0 if parsing fails.
func MajorVersion(s string) int {
	v, err := ParseVersion(s)
	if err != nil {
		return 0
	}
	return v.Major
}

// knownLTSVersions lists Qt versions with LTS status.
var knownLTSVersions = map[string]bool{
	"5.9":  true,
	"5.12": true,
	"5.15": true,
	"6.2":  true,
	"6.5":  true,
	"6.8":  true,
}

// IsLTS reports whether a version is a known LTS release.
func IsLTS(s string) bool {
	v, err := ParseVersion(s)
	if err != nil {
		return false
	}
	key := fmt.Sprintf("%d.%d", v.Major, v.Minor)
	return knownLTSVersions[key]
}
