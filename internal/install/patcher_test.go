package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchQtConf(t *testing.T) {
	tests := []struct {
		name           string
		existingConf   *string // nil = no existing file; pointer to content otherwise
		createBinDir   bool    // whether to pre-create the bin directory
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:         "creates new qt.conf when none exists",
			existingConf: nil,
			createBinDir: false,
			wantContains: []string{
				"[Paths]",
				"Prefix=",
			},
		},
		{
			name:         "updates existing with Prefix line",
			existingConf: strPtr("[Paths]\nPrefix=/old/path\nTranslations=translations\n"),
			createBinDir: true,
			wantContains: []string{
				"[Paths]",
				"Translations=translations",
			},
			wantNotContain: []string{
				"Prefix=/old/path",
			},
		},
		{
			name:         "inserts Prefix after [Paths] when missing",
			existingConf: strPtr("[Paths]\nTranslations=translations\nPlugins=plugins\n"),
			createBinDir: true,
			wantContains: []string{
				"[Paths]",
				"Prefix=",
				"Translations=translations",
				"Plugins=plugins",
			},
		},
		{
			name:         "prepends [Paths] section when absent",
			existingConf: strPtr("Translations=translations\nPlugins=plugins\n"),
			createBinDir: true,
			wantContains: []string{
				"[Paths]",
				"Prefix=",
				"Translations=translations",
				"Plugins=plugins",
			},
		},
		{
			name:         "creates bin directory if needed",
			existingConf: nil,
			createBinDir: false,
			wantContains: []string{
				"[Paths]",
				"Prefix=",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installDir := t.TempDir()
			binDir := filepath.Join(installDir, "bin")
			qtConfPath := filepath.Join(binDir, "qt.conf")

			if tt.createBinDir {
				require.NoError(t, os.MkdirAll(binDir, 0o755))
			}

			if tt.existingConf != nil {
				require.NoError(t, os.MkdirAll(binDir, 0o755))
				require.NoError(t, os.WriteFile(qtConfPath, []byte(*tt.existingConf), 0o644))
			}

			err := PatchQtConf(installDir)
			require.NoError(t, err)

			// Verify bin dir exists.
			fi, err := os.Stat(binDir)
			require.NoError(t, err)
			assert.True(t, fi.IsDir())

			// Verify qt.conf content.
			data, err := os.ReadFile(qtConfPath)
			require.NoError(t, err)
			content := string(data)

			expectedPrefix := filepath.ToSlash(installDir)
			assert.Contains(t, content, "Prefix="+expectedPrefix)

			for _, want := range tt.wantContains {
				assert.Contains(t, content, want)
			}
			for _, notWant := range tt.wantNotContain {
				assert.NotContains(t, content, notWant)
			}
		})
	}
}

func TestPatchQtConf_PrefixUsesForwardSlashes(t *testing.T) {
	installDir := t.TempDir()

	err := PatchQtConf(installDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(installDir, "bin", "qt.conf"))
	require.NoError(t, err)

	content := string(data)
	expectedPrefix := filepath.ToSlash(installDir)
	assert.Contains(t, content, "Prefix="+expectedPrefix)
	// On Windows, verify no backslashes in the Prefix value.
	assert.NotContains(t, content, "Prefix="+installDir+"\\")
}

func strPtr(s string) *string {
	return &s
}
