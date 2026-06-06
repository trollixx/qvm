package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trollixx/qvm/internal/storage"
)

const testPrefix = `C:\Qt\6.10.2\msvc2022_64`

// fakeGetenv returns a getenv func backed by a map.
func fakeGetenv(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestBuildQtEnv_EmptyEnvironment(t *testing.T) {
	env := buildQtEnv(testPrefix, fakeGetenv(nil))

	assert.Equal(t, testPrefix, env["CMAKE_PREFIX_PATH"], "no dangling separator on empty value")
	assert.Equal(t, filepath.Join(testPrefix, "bin"), env["PATH"])
}

func TestBuildQtEnv_PrependsToExistingValues(t *testing.T) {
	sep := string(os.PathListSeparator)
	env := buildQtEnv(testPrefix, fakeGetenv(map[string]string{
		"CMAKE_PREFIX_PATH": `C:\vcpkg\installed\x64-windows`,
		"PATH":              `C:\Windows\system32`,
	}))

	assert.Equal(t, testPrefix+sep+`C:\vcpkg\installed\x64-windows`, env["CMAKE_PREFIX_PATH"])
	assert.Equal(t, filepath.Join(testPrefix, "bin")+sep+`C:\Windows\system32`, env["PATH"])
}

func TestWriteEnvScript_Formats(t *testing.T) {
	env := map[string]string{"PATH": `C:\Qt\bin`, "CMAKE_PREFIX_PATH": `C:\Qt`}

	tests := []struct {
		shell string
		want  []string
	}{
		{"powershell", []string{"$env:CMAKE_PREFIX_PATH = 'C:\\Qt'", "$env:PATH = 'C:\\Qt\\bin'"}},
		{"pwsh", []string{"$env:PATH = 'C:\\Qt\\bin'"}},
		{"cmd", []string{`set CMAKE_PREFIX_PATH=C:\Qt`, `set PATH=C:\Qt\bin`}},
		{"bash", []string{"export CMAKE_PREFIX_PATH='C:\\Qt'", "export PATH='C:\\Qt\\bin'"}},
		{"zsh", []string{"export PATH='C:\\Qt\\bin'"}},
		{"fish", []string{"set -gx CMAKE_PREFIX_PATH 'C:\\Qt'", "set -gx PATH 'C:\\Qt\\bin'"}},
	}
	for _, tc := range tests {
		t.Run(tc.shell, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, writeEnvScript(&buf, tc.shell, env))
			for _, line := range tc.want {
				assert.Contains(t, buf.String(), line+"\n")
			}
		})
	}
}

func TestWriteEnvScript_SortedAndCaseInsensitiveShell(t *testing.T) {
	var buf bytes.Buffer
	env := map[string]string{"PATH": "b", "CMAKE_PREFIX_PATH": "a"}
	require.NoError(t, writeEnvScript(&buf, "PowerShell", env))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "CMAKE_PREFIX_PATH", "keys must be emitted in sorted order")
	assert.Contains(t, lines[1], "PATH")
}

func TestWriteEnvScript_Nu_EmitsJSON(t *testing.T) {
	var buf bytes.Buffer
	env := map[string]string{"PATH": `C:\Qt\bin`, "CMAKE_PREFIX_PATH": `C:\Qt`}
	require.NoError(t, writeEnvScript(&buf, "nu", env))

	var decoded map[string]string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, env, decoded)
}

func TestWriteEnvScript_Quoting(t *testing.T) {
	env := map[string]string{"PATH": `C:\it's here`}

	var buf bytes.Buffer
	require.NoError(t, writeEnvScript(&buf, "powershell", env))
	assert.Contains(t, buf.String(), `$env:PATH = 'C:\it''s here'`)

	buf.Reset()
	require.NoError(t, writeEnvScript(&buf, "bash", env))
	assert.Contains(t, buf.String(), `export PATH='C:\it'\''s here'`)
}

func TestWriteEnvScript_UnsupportedShell(t *testing.T) {
	err := writeEnvScript(&bytes.Buffer{}, "tcsh", map[string]string{"PATH": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tcsh")
}

func TestResolveShell(t *testing.T) {
	assert.Equal(t, "fish", resolveShell("fish"))
	if runtime.GOOS == "windows" {
		assert.Equal(t, "powershell", resolveShell(""))
	} else {
		assert.Equal(t, "bash", resolveShell(""))
	}
}

func TestEnvQt_NotInstalled(t *testing.T) {
	a, _ := newTestApp()
	err := a.envQt(&storage.Registry{}, "6.8.3", "", "bash")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not installed")
}

func TestEnvQt_EmitsExports(t *testing.T) {
	a, out := newTestApp()
	reg := &storage.Registry{
		Qt: []storage.InstalledQt{
			{Version: "6.10.2", Arch: "win64_msvc2022_64", InstallDir: testPrefix},
		},
	}
	err := a.envQt(reg, "6.10.2", "", "bash")
	require.NoError(t, err)

	assert.Contains(t, out.String(), "export CMAKE_PREFIX_PATH='"+testPrefix)
	assert.Contains(t, out.String(), "export PATH='"+filepath.Join(testPrefix, "bin"))
}
