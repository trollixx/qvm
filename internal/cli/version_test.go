package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveVersion_LdflagsOverrideWins(t *testing.T) {
	assert.Equal(t, "1.2.3", resolveVersion("1.2.3"))
}

func TestResolveVersion_DevFallsBackToBuildInfo(t *testing.T) {
	// Test binaries carry no usable module version ("(devel)" or empty),
	// so "dev" must pass through unchanged rather than become "(devel)".
	assert.Equal(t, "dev", resolveVersion("dev"))
}

func TestVersionDisplay(t *testing.T) {
	tests := []struct{ in, want string }{
		{"dev", "dev"},
		{"1.2.3", "v1.2.3"},
		{"v1.2.3", "v1.2.3"},
		{"v0.0.0-20260607120000-abcdef123456", "v0.0.0-20260607120000-abcdef123456"},
		{"", ""},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, versionDisplay(tc.in), "input %q", tc.in)
	}
}
