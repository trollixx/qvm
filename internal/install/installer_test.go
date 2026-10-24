package install

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trollixx/qvm/internal/storage"
)

func TestDiffSlices(t *testing.T) {
	tests := []struct {
		name      string
		requested []string
		installed []string
		want      []string
	}{
		{
			name:      "all new",
			requested: []string{"a", "b", "c"},
			installed: []string{},
			want:      []string{"a", "b", "c"},
		},
		{
			name:      "none new",
			requested: []string{"a", "b"},
			installed: []string{"a", "b"},
			want:      nil,
		},
		{
			name:      "some new",
			requested: []string{"a", "b", "c"},
			installed: []string{"a"},
			want:      []string{"b", "c"},
		},
		{
			name:      "empty requested",
			requested: []string{},
			installed: []string{"a", "b"},
			want:      nil,
		},
		{
			name:      "both empty",
			requested: nil,
			installed: nil,
			want:      nil,
		},
		{
			name:      "nil requested",
			requested: nil,
			installed: []string{"a"},
			want:      nil,
		},
		{
			name:      "duplicates in requested",
			requested: []string{"a", "a", "b"},
			installed: []string{"a"},
			want:      []string{"b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffSlices(tt.requested, tt.installed)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMergeSlices(t *testing.T) {
	tests := []struct {
		name      string
		existing  []string
		additions []string
		want      []string
	}{
		{
			name:      "merge disjoint",
			existing:  []string{"a", "b"},
			additions: []string{"c", "d"},
			want:      []string{"a", "b", "c", "d"},
		},
		{
			name:      "merge overlapping",
			existing:  []string{"a", "b"},
			additions: []string{"b", "c"},
			want:      []string{"a", "b", "c"},
		},
		{
			name:      "merge identical",
			existing:  []string{"a", "b"},
			additions: []string{"a", "b"},
			want:      []string{"a", "b"},
		},
		{
			name:      "empty existing",
			existing:  nil,
			additions: []string{"a", "b"},
			want:      []string{"a", "b"},
		},
		{
			name:      "empty additions",
			existing:  []string{"a", "b"},
			additions: nil,
			want:      []string{"a", "b"},
		},
		{
			name:      "both empty",
			existing:  nil,
			additions: nil,
			want:      []string{},
		},
		{
			name:      "preserves existing order",
			existing:  []string{"c", "a", "b"},
			additions: []string{"d", "a"},
			want:      []string{"c", "a", "b", "d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeSlices(tt.existing, tt.additions)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeModuleNames(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "already prefixed",
			input: []string{"qtcharts", "qtwebengine"},
			want:  []string{"qtcharts", "qtwebengine"},
		},
		{
			name:  "missing prefix",
			input: []string{"charts", "webengine"},
			want:  []string{"qtcharts", "qtwebengine"},
		},
		{
			name:  "mixed",
			input: []string{"webengine", "qtcharts", "httpserver"},
			want:  []string{"qtwebengine", "qtcharts", "qthttpserver"},
		},
		{
			name:  "empty",
			input: nil,
			want:  []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeModuleNames(tt.input))
		})
	}
}

func TestDiffSlices_WithNormalizedModules(t *testing.T) {
	// Simulates the real scenario: user passes "webengine", registry has "qtwebengine".
	// After normalization, diffSlices should see no difference.
	requested := normalizeModuleNames([]string{"webengine", "imageformats", "httpserver"})
	installed := []string{"qtwebengine", "qtimageformats", "qthttpserver"}
	assert.Nil(t, diffSlices(requested, installed))
}

func TestDiffSlices_NormalizedWithNewModule(t *testing.T) {
	// User requests modules including one that's not yet installed.
	requested := normalizeModuleNames([]string{"webengine", "charts"})
	installed := []string{"qtwebengine"}
	got := diffSlices(requested, installed)
	assert.Equal(t, []string{"qtcharts"}, got)
}

func TestBuildRegistryEntry_NormalizesModules(t *testing.T) {
	t.Run("fresh install canonicalizes raw module names", func(t *testing.T) {
		opts := Options{
			Version: "6.8.3",
			Arch:    "win64_msvc2022_64",
			Modules: []string{"webengine", "imageformats"},
		}
		entry := buildRegistryEntry(opts, nil, opts.Modules, "C:\\Qt\\6.8.3\\win64_msvc2022_64", 0)
		assert.Equal(t, []string{"qtwebengine", "qtimageformats"}, entry.Modules)
	})

	t.Run("delta install canonicalizes new module names without re-adding existing", func(t *testing.T) {
		existing := &storage.InstalledQt{
			Version: "6.8.3",
			Arch:    "win64_msvc2022_64",
			Modules: []string{"qtwebengine"},
		}
		opts := Options{
			Version: "6.8.3",
			Arch:    "win64_msvc2022_64",
			Modules: []string{"webengine", "charts"}, // raw user input
		}
		entry := buildRegistryEntry(opts, existing, opts.Modules, "C:\\Qt\\6.8.3\\win64_msvc2022_64", 0)
		assert.Equal(t, []string{"qtwebengine", "qtcharts"}, entry.Modules)
	})
}

func TestBuildRegistryEntry_RecordsDependencyModules(t *testing.T) {
	opts := Options{
		Version: "6.11.1",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"httpserver"},
	}
	// Resolver returned the requested module plus an auto-added dependency.
	resolved := []string{"httpserver", "qtwebsockets"}
	entry := buildRegistryEntry(opts, nil, resolved, "C:\\Qt\\6.11.1\\win64_msvc2022_64", 0)
	assert.Equal(t, []string{"qthttpserver", "qtwebsockets"}, entry.Modules)
}

func TestInstallDirFor(t *testing.T) {
	const version, arch = "6.8.3", "win64_msvc2022_64"
	registeredRoot, otherRoot := t.TempDir(), t.TempDir()
	registered := storage.InstallDir(registeredRoot, version, arch)

	reg := storage.NewRegistryManagerAt(filepath.Join(t.TempDir(), "registry.json"))
	require.NoError(t, reg.AddQt(storage.InstalledQt{Version: version, Arch: arch, InstallDir: registered}))
	inst := NewInstaller(nil, reg)

	t.Run("an install stays in its directory without --dir", func(t *testing.T) {
		dir, err := inst.installDirFor(Options{Version: version, Arch: arch, InstallRoot: otherRoot})
		require.NoError(t, err)
		assert.Equal(t, registered, dir)
	})

	t.Run("--dir elsewhere fails, even with --force", func(t *testing.T) {
		for _, force := range []bool{false, true} {
			_, err := inst.installDirFor(Options{
				Version: version, Arch: arch, InstallRoot: otherRoot, RootSet: true, Force: force,
			})
			var elsewhere *InstalledElsewhereError
			require.ErrorAs(t, err, &elsewhere)
			assert.Equal(t, registered, elsewhere.Dir)
			assert.Equal(t, storage.InstallDir(otherRoot, version, arch), elsewhere.Requested)
		}
	})

	t.Run("--dir naming its root is fine", func(t *testing.T) {
		dir, err := inst.installDirFor(Options{
			Version: version, Arch: arch, InstallRoot: registeredRoot, RootSet: true,
		})
		require.NoError(t, err)
		assert.Equal(t, registered, dir)
	})

	t.Run("another arch goes where asked", func(t *testing.T) {
		dir, err := inst.installDirFor(Options{Version: version, Arch: "win64_mingw", InstallRoot: otherRoot})
		require.NoError(t, err)
		assert.Equal(t, storage.InstallDir(otherRoot, version, "win64_mingw"), dir)
	})

	t.Run("a relative root is made absolute", func(t *testing.T) {
		dir, err := inst.installDirFor(Options{Version: version, Arch: "win64_mingw", InstallRoot: "Qt", RootSet: true})
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(dir), dir)
	})
}

func TestBuildResolveOptions_NoDeps(t *testing.T) {
	ro, _ := buildResolveOptions(Options{NoDeps: true}, nil, nil)
	assert.True(t, ro.NoDeps)

	ro, _ = buildResolveOptions(Options{NoDeps: true}, &storage.InstalledQt{}, nil)
	assert.True(t, ro.NoDeps, "delta installs must honor NoDeps too")
}

func TestBuildResolveOptions_InstalledModules(t *testing.T) {
	existing := &storage.InstalledQt{Modules: []string{"qtwebengine"}}
	modules := []string{"qtwebengine"}

	// Resolved, so the resolver can add dependencies the install lacks.
	ro, upToDate := buildResolveOptions(Options{}, existing, modules)
	assert.False(t, upToDate)
	assert.Equal(t, modules, ro.Modules)
	assert.Equal(t, existing.Modules, ro.Installed)

	// Without dependencies, an installed module needs nothing.
	ro, upToDate = buildResolveOptions(Options{NoDeps: true}, existing, modules)
	assert.True(t, upToDate)
	assert.Empty(t, ro.Modules)

	// Nor does asking for no modules, which needs no metadata.
	_, upToDate = buildResolveOptions(Options{}, existing, nil)
	assert.True(t, upToDate)
}

func TestReportDepModules(t *testing.T) {
	t.Run("emits info event for added dependencies", func(t *testing.T) {
		ch := make(chan ProgressEvent, 1)
		reportDepModules(ch, []string{"httpserver"}, []string{"httpserver", "qtwebsockets"})

		ev := <-ch
		assert.Equal(t, "info", ev.Phase)
		assert.Contains(t, ev.Message, "qtwebsockets")
		assert.NotContains(t, ev.Message, "qthttpserver")
	})

	t.Run("silent when nothing was added", func(t *testing.T) {
		ch := make(chan ProgressEvent, 1)
		reportDepModules(ch, []string{"charts"}, []string{"qtcharts"})
		assert.Empty(t, ch)
	})
}

func TestSendProgress(t *testing.T) {
	t.Run("nil channel does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			sendProgress(nil, ProgressEvent{Phase: "testing"})
		})
	})

	t.Run("full channel does not block", func(_ *testing.T) {
		ch := make(chan ProgressEvent) // unbuffered, no receiver
		done := make(chan struct{})
		go func() {
			sendProgress(ch, ProgressEvent{Phase: "testing"})
			close(done)
		}()
		// If sendProgress blocks, this test will time out.
		<-done
	})

	t.Run("normal channel receives event", func(t *testing.T) {
		ch := make(chan ProgressEvent, 1)
		ev := ProgressEvent{Phase: "downloading", Percent: 50}
		sendProgress(ch, ev)

		received := <-ch
		assert.Equal(t, "downloading", received.Phase)
		assert.InEpsilon(t, float64(50), received.Percent, 1e-9)
	})
}
