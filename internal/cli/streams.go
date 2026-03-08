package cli

import (
	"bytes"
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// IOStreams bundles the three standard I/O streams used by all CLI commands.
// Inject test-specific buffers via NewTestIOStreams for unit testing.
type IOStreams struct {
	In     io.Reader // for confirm() prompts
	Out    io.Writer // user-facing output
	ErrOut io.Writer // progress/status messages
	isTTY  bool      // whether ErrOut is a real terminal
}

// NewIOStreams creates an IOStreams wired to the real file descriptors.
func NewIOStreams() *IOStreams {
	return &IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
		isTTY:  isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()),
	}
}

// NewTestIOStreams returns an IOStreams backed by in-memory buffers.
// outBuf and errBuf can be inspected after running a command.
func NewTestIOStreams() (*IOStreams, *bytes.Buffer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	return &IOStreams{
		In:     &bytes.Buffer{},
		Out:    out,
		ErrOut: errOut,
		isTTY:  false,
	}, out, errOut
}

// NewProgressPrinter creates a fresh progressPrinter writing to s.ErrOut.
func (s *IOStreams) NewProgressPrinter() *progressPrinter {
	return &progressPrinter{
		isTTY: s.isTTY,
		w:     s.ErrOut,
	}
}
