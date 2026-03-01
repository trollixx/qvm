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
		{"5.15.18", 5, 15, 18, false},
		{"6.8.3", 6, 8, 3, false},
		{"6.0.0", 6, 0, 0, false},
		{"5.9.0", 5, 9, 0, false},
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
			assert.Equal(t, tc.wantMajor, v.Major)
			assert.Equal(t, tc.wantMinor, v.Minor)
			assert.Equal(t, tc.wantPatch, v.Patch)
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
		{"5.15.18", "6.0.0", true},
		{"6.0.0", "5.15.18", false},
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
	lts := []string{"6.8.0", "6.8.3", "6.5.0", "6.5.3", "6.2.0", "5.15.18", "5.12.0", "5.9.0"}
	notLTS := []string{"6.10.0", "6.7.3", "6.6.0", "6.4.0"}

	for _, v := range lts {
		assert.True(t, IsLTS(v), "expected %s to be LTS", v)
	}
	for _, v := range notLTS {
		assert.False(t, IsLTS(v), "expected %s to not be LTS", v)
	}
}

func TestMajorVersion(t *testing.T) {
	assert.Equal(t, 6, MajorVersion("6.10.0"))
	assert.Equal(t, 5, MajorVersion("5.15.18"))
	assert.Equal(t, 0, MajorVersion("bad"))
}
