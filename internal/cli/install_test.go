package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/trollixx/qvm/internal/install"
	"github.com/trollixx/qvm/internal/platform"
)

func TestPrintDryRun(t *testing.T) {
	a, out := newTestApp()
	for _, ev := range []install.ProgressEvent{
		{Phase: "dryrun", ArchiveTotal: 2, BytesTotal: 3072},
		{Phase: "dryrun", Archive: "a.7z", BytesTotal: 1024},
		{Phase: "dryrun", Archive: "b.7z"}, // size unknown
		{Phase: "dryrun", Percent: 100, BytesTotal: 3072},
	} {
		a.printDryRun(ev)
	}
	assert.Equal(t, "Dry run - would download 2 archive(s) (3.0 KB total):\n"+
		"  a.7z  1.0 KB\n"+
		"  b.7z\n"+
		"\nTotal: 3.0 KB\n", out.String())
}

// Without sizes, there's no total in the header or at the end.
func TestPrintDryRun_NoSizes(t *testing.T) {
	a, out := newTestApp()
	a.printDryRun(install.ProgressEvent{Phase: "dryrun", ArchiveTotal: 1})
	a.printDryRun(install.ProgressEvent{Phase: "dryrun", Archive: "a.7z"})
	a.printDryRun(install.ProgressEvent{Phase: "dryrun", Percent: 100})
	assert.Equal(t, "Dry run - would download 1 archive(s):\n  a.7z\n", out.String())
}

func TestFormatDownloadLine(t *testing.T) {
	assert.Equal(t, "  a.7z  50% @ 2.0 KB/s",
		formatDownloadLine(install.ProgressEvent{Archive: "a.7z", BytesDone: 512, BytesTotal: 1024, Speed: 2048}))
	assert.Equal(t, "  a.7z", formatDownloadLine(install.ProgressEvent{Archive: "a.7z"}))
}

// Without a terminal, each download is reported once, when it completes, and
// per-file extraction updates are left out.
func TestProgressPrinter_NotTTY(t *testing.T) {
	var buf bytes.Buffer
	p := &ProgressPrinter{w: &buf}
	for _, ev := range []install.ProgressEvent{
		{Phase: "resolving"},
		{Phase: "downloading"},
		{Phase: "downloading", Archive: "a.7z", BytesDone: 5, BytesTotal: 10},
		{Phase: "downloading", Archive: "a.7z", BytesDone: 10, BytesTotal: 10},
		{Phase: "downloading", Archive: "a.7z", BytesDone: 10, BytesTotal: 10},
		{Phase: "extracting"},
		{Phase: "extracting", Archive: "a.7z", BytesDone: 1, BytesTotal: 2},
		{Phase: "warning", Warning: "careful"},
		{Phase: "done"},
	} {
		p.print(ev)
	}
	assert.Equal(t, "Resolving archives...\n"+
		"  Downloaded a.7z\n"+
		"Extracting archives...\n"+
		"warning: careful\n"+
		"Done.\n", buf.String())
}

// In a terminal, each download has a line that's rewritten in place.
func TestProgressPrinter_TTY(t *testing.T) {
	var buf bytes.Buffer
	p := &ProgressPrinter{isTTY: true, w: &buf}
	p.print(install.ProgressEvent{Phase: "downloading"})
	p.print(install.ProgressEvent{Phase: "downloading", Archive: "a.7z", BytesDone: 1, BytesTotal: 2})
	p.print(install.ProgressEvent{Phase: "downloading", Archive: "b.7z", BytesDone: 1, BytesTotal: 4})
	p.print(install.ProgressEvent{Phase: "downloading", Archive: "a.7z", BytesDone: 2, BytesTotal: 2})
	assert.Equal(t, "  a.7z  50%\n"+
		"  b.7z  25%\n"+
		"\033[2A\r\033[K  a.7z 100%\033[2B\r", buf.String())
}

// --host picks that host's default, as for install and list-remote <version>.
func TestDefaultArch(t *testing.T) {
	assert.Equal(t, "win64_msvc2022_arm64", defaultArch("6.8.3", "windows_arm64"))
	assert.Equal(t, "win64_msvc2019_64", defaultArch("6.7.3", "windows_x86"))
	assert.Equal(t, "clang_64", defaultArch("6.8.3", "mac_x64"))

	// Without --host, or with one that has no default, it's this machine's.
	assert.Equal(t, platform.Current().DefaultArch("6.8.3"), defaultArch("6.8.3", ""))
	assert.Equal(t, platform.Current().DefaultArch("6.8.3"), defaultArch("6.8.3", "all_os"))
}
