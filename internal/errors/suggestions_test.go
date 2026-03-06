package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuggestModule_WithSuggestions(t *testing.T) {
	available := []string{"qtcharts", "qtwebengine", "qtpdf", "qt3d"}
	err := SuggestModule("charts", available)
	require.NotNil(t, err)
	assert.Equal(t, CodeUnknownModule, err.Code)
	assert.Contains(t, err.Error(), `module "charts" is not known`)
	assert.Contains(t, err.Error(), "Did you mean")
	assert.Contains(t, err.Error(), "qtcharts")
}

func TestSuggestModule_NoSuggestions(t *testing.T) {
	err := SuggestModule("zzzzz", []string{"qtcharts"})
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "qvm list")
}

func TestSuggestArch_HasSuggestions(t *testing.T) {
	available := []string{"win64_msvc2022_64", "win64_msvc2022_arm64", "win64_mingw"}
	err := SuggestArch("win64_msvc2022", available)
	require.NotNil(t, err)
	assert.Equal(t, CodeUnknownArch, err.Code)
	assert.Contains(t, err.Error(), "Did you mean")
}

func TestSuggestVersion_HasSuggestions(t *testing.T) {
	available := []string{"6.8.3", "6.10.1", "6.10.2"}
	err := SuggestVersion("6.10", available)
	require.NotNil(t, err)
	assert.Equal(t, CodeUnknownVersion, err.Code)
	assert.Contains(t, err.Error(), "Did you mean")
}

func TestSuggestClose_Empty(t *testing.T) {
	assert.Empty(t, SuggestClose("foo", nil, 3))
	assert.Empty(t, SuggestClose("foo", []string{}, 3))
}
