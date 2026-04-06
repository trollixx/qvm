package qtmeta

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input     string
		wantMajor int
		wantMinor int
		wantPatch int
		wantErr   bool
	}{
		{"6.10.0", 6, 10, 0, false},
		{"6.8.3", 6, 8, 3, false},
		{"6.0.0", 6, 0, 0, false},
		{"", 0, 0, 0, true},
		{"6.10", 0, 0, 0, true},
		{"abc", 0, 0, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			v, err := ParseVersion(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantMajor, v.Major())
			assert.Equal(t, tc.wantMinor, v.Minor())
			assert.Equal(t, tc.wantPatch, v.Patch())
		})
	}
}

func TestVersionLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"6.8.3", "6.10.0", true},
		{"6.10.0", "6.8.3", false},
		{"6.8.3", "6.8.3", false},
		{"6.0.0", "6.8.3", true},
		{"6.8.0", "6.8.1", true},
	}
	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			a, err := ParseVersion(tc.a)
			require.NoError(t, err)
			b, err := ParseVersion(tc.b)
			require.NoError(t, err)
			assert.Equal(t, tc.want, a.Less(b))
		})
	}
}

func TestIsLTS(t *testing.T) {
	lts := []string{"6.8.0", "6.8.3", "6.5.0", "6.5.3", "6.2.0"}
	notLTS := []string{"6.10.0", "6.7.3", "6.6.0", "6.4.0"}

	for _, v := range lts {
		assert.True(t, IsLTS(v), "expected %s to be LTS", v)
	}
	for _, v := range notLTS {
		assert.False(t, IsLTS(v), "expected %s to not be LTS", v)
	}
}

func TestParseVersionFilter(t *testing.T) {
	tests := []struct {
		input    string
		wantStr  string
		wantFull bool
		wantErr  bool
	}{
		{"6", "6", false, false},
		{"6.9", "6.9", false, false},
		{"6.9.0", "6.9.0", true, false},
		{"6.2.0", "6.2.0", true, false},
		{"", "", false, true},
		{"abc", "", false, true},
		{"6.abc", "", false, true},
		{"6.9.abc", "", false, true},
		{"1.2.3.4", "", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			vf, err := ParseVersionFilter(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantStr, vf.String())
			assert.Equal(t, tc.wantFull, vf.IsFullVersion())
		})
	}
}

func TestVersionFilterMatches(t *testing.T) {
	tests := []struct {
		filter  string
		version string
		want    bool
	}{
		// Major-only filter.
		{"6", "6.8.3", true},
		{"6", "6.10.0", true},
		{"6", "7.0.0", false},
		// Major.minor filter.
		{"6.8", "6.8.0", true},
		{"6.8", "6.8.3", true},
		{"6.8", "6.9.0", false},
		{"6.8", "7.8.0", false},
		// Full version filter.
		{"6.8.3", "6.8.3", true},
		{"6.8.3", "6.8.0", false},
	}
	for _, tc := range tests {
		t.Run(tc.filter+"_vs_"+tc.version, func(t *testing.T) {
			vf, err := ParseVersionFilter(tc.filter)
			require.NoError(t, err)
			assert.Equal(t, tc.want, vf.MatchesString(tc.version))
		})
	}
}

func TestMajorVersion(t *testing.T) {
	assert.Equal(t, 6, MajorVersion("6.10.0"))
	assert.Equal(t, 6, MajorVersion("6.5.3"))
	assert.Equal(t, 0, MajorVersion("bad"))
}
