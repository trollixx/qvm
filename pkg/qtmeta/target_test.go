package qtmeta

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLookupTarget_Known(t *testing.T) {
	meta, ok := LookupTarget("win64_msvc2022_64")
	assert.True(t, ok)
	assert.Equal(t, TargetMeta{Name: "win64_msvc2022_64", DisplayName: "MSVC 2022, 64-bit", OS: Windows}, meta)
}

// An unknown arch is named after itself, with its OS guessed from the name.
func TestLookupTarget_Unknown(t *testing.T) {
	meta, ok := LookupTarget("win64_msvc2019_arm64")
	assert.False(t, ok)
	assert.Equal(t, TargetMeta{Name: "win64_msvc2019_arm64", DisplayName: "win64_msvc2019_arm64", OS: Windows}, meta)
}

func TestGuessOS(t *testing.T) {
	tests := []struct {
		name string
		want TargetOS
	}{
		{"win64_llvm_mingw", Windows},
		{"WIN64_MSVC2022_64", Windows},
		{"linux_gcc_arm64", Linux},
		{"gcc_64", Linux},
		{"clang_64", MacOS},
		{"macos", MacOS},
		{"android_arm64_v8a", Linux}, // no hint in the name
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, guessOS(tc.name))
		})
	}
}
