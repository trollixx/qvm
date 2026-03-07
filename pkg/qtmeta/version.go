package qtmeta

import (
	"fmt"
	"strconv"
	"strings"

	goversion "github.com/hashicorp/go-version"
)

// Version represents a parsed Qt version number.
type Version struct {
	v                   *goversion.Version
	major, minor, patch int
}

// ParseVersion parses a Qt version string like "6.10.0" or "5.15.18".
// Requires exactly three dot-separated numeric components.
func ParseVersion(s string) (Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: expected major.minor.patch", s)
	}
	v, err := goversion.NewVersion(s)
	if err != nil {
		return Version{}, fmt.Errorf("invalid version %q: %w", s, err)
	}
	segs := v.Segments()
	return Version{v: v, major: segs[0], minor: segs[1], patch: segs[2]}, nil
}

// MustParseVersion is like ParseVersion but panics on error.
func MustParseVersion(s string) Version {
	v, err := ParseVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

// Major returns the major version number.
func (v Version) Major() int { return v.major }

// Minor returns the minor version number.
func (v Version) Minor() int { return v.minor }

// Patch returns the patch version number.
func (v Version) Patch() int { return v.patch }

// String returns the version as "major.minor.patch".
func (v Version) String() string {
	return v.v.String()
}

// Compare returns negative, zero, or positive depending on whether v < other, v == other, or v > other.
func (v Version) Compare(other Version) int {
	return v.v.Compare(other.v)
}

// Less reports whether v is older than other.
func (v Version) Less(other Version) bool {
	return v.v.LessThan(other.v)
}

// GTE reports whether v is greater than or equal to other.
func (v Version) GTE(other Version) bool {
	return v.v.GreaterThanOrEqual(other.v)
}

// MajorVersion returns the major version number from a version string.
// Returns 0 if parsing fails.
func MajorVersion(s string) int {
	v, err := ParseVersion(s)
	if err != nil {
		return 0
	}
	return v.Major()
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
		var minor int
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return VersionFilter{}, fmt.Errorf("invalid minor version in %q: %w", s, err)
		}
		vf.Minor = minor
		vf.HasMinor = true
	}

	if len(parts) == 3 {
		var patch int
		patch, err = strconv.Atoi(parts[2])
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
	if v.Major() != vf.Major {
		return false
	}
	if vf.HasMinor && v.Minor() != vf.Minor {
		return false
	}
	if vf.HasPatch && v.Patch() != vf.Patch {
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

// IsLTS reports whether a version is a known LTS release.
func IsLTS(s string) bool {
	ltsVersions := map[string]bool{
		"5.9":  true,
		"5.12": true,
		"5.15": true,
		"6.2":  true,
		"6.5":  true,
		"6.8":  true,
	}
	v, err := ParseVersion(s)
	if err != nil {
		return false
	}
	key := fmt.Sprintf("%d.%d", v.Major(), v.Minor())
	return ltsVersions[key]
}
