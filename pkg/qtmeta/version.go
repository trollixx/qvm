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

// VersionFilter represents a partial or full version specification used for
// filtering. It can match major-only ("6"), major.minor ("6.9"), or exact
// major.minor.patch ("6.9.0").
type VersionFilter struct {
	Major    int
	Minor    int
	Patch    int
	HasMinor bool
	HasPatch bool
}

// ParseVersionFilter parses a partial or full version string.
// Accepted formats: "6", "6.9", "6.9.0".
func ParseVersionFilter(s string) (VersionFilter, error) {
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return VersionFilter{}, fmt.Errorf("invalid version filter %q: expected major[.minor[.patch]]", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return VersionFilter{}, fmt.Errorf("invalid major version in %q: %w", s, err)
	}

	vf := VersionFilter{Major: major}

	if len(parts) >= 2 {
		minor, err := strconv.Atoi(parts[1])
		if err != nil {
			return VersionFilter{}, fmt.Errorf("invalid minor version in %q: %w", s, err)
		}
		vf.Minor = minor
		vf.HasMinor = true
	}

	if len(parts) == 3 {
		patch, err := strconv.Atoi(parts[2])
		if err != nil {
			return VersionFilter{}, fmt.Errorf("invalid patch version in %q: %w", s, err)
		}
		vf.Patch = patch
		vf.HasPatch = true
	}

	return vf, nil
}

// IsFullVersion returns true if the filter specifies all three components.
func (vf VersionFilter) IsFullVersion() bool {
	return vf.HasMinor && vf.HasPatch
}

// Matches reports whether a parsed Version matches this filter.
func (vf VersionFilter) Matches(v Version) bool {
	if v.Major != vf.Major {
		return false
	}
	if vf.HasMinor && v.Minor != vf.Minor {
		return false
	}
	if vf.HasPatch && v.Patch != vf.Patch {
		return false
	}
	return true
}

// MatchesString reports whether a version string like "6.9.0" matches this filter.
func (vf VersionFilter) MatchesString(version string) bool {
	v, err := ParseVersion(version)
	if err != nil {
		return false
	}
	return vf.Matches(v)
}

// String returns the filter as originally specified (e.g. "6", "6.9", "6.9.0").
func (vf VersionFilter) String() string {
	if vf.HasPatch {
		return fmt.Sprintf("%d.%d.%d", vf.Major, vf.Minor, vf.Patch)
	}
	if vf.HasMinor {
		return fmt.Sprintf("%d.%d", vf.Major, vf.Minor)
	}
	return strconv.Itoa(vf.Major)
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
