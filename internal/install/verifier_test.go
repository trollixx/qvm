package install

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sha1Hex(data []byte) string {
	h := sha1.Sum(data)
	return hex.EncodeToString(h[:])
}

func TestVerifyFile(t *testing.T) {
	tests := []struct {
		name         string
		content      []byte  // file content; nil means don't create the file
		expectedSHA1 string  // empty = skip verification
		wantErr      bool
		errContains  string
	}{
		{
			name:         "matching SHA1",
			content:      []byte("hello world"),
			expectedSHA1: sha1Hex([]byte("hello world")),
			wantErr:      false,
		},
		{
			name:         "mismatched SHA1",
			content:      []byte("hello world"),
			expectedSHA1: "0000000000000000000000000000000000000000",
			wantErr:      true,
			errContains:  "checksum mismatch",
		},
		{
			name:         "empty expected SHA1 skips verification",
			content:      []byte("anything"),
			expectedSHA1: "",
			wantErr:      false,
		},
		{
			name:         "empty file matches its own SHA1",
			content:      []byte{},
			expectedSHA1: sha1Hex([]byte{}),
			wantErr:      false,
		},
		{
			name:         "missing file",
			content:      nil,
			expectedSHA1: "da39a3ee5e6b4b0d3255bfef95601890afd80709",
			wantErr:      true,
			errContains:  "opening",
		},
		{
			name:         "binary content",
			content:      []byte{0x00, 0xFF, 0x0A, 0x0D, 0x80},
			expectedSHA1: sha1Hex([]byte{0x00, 0xFF, 0x0A, 0x0D, 0x80}),
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "testfile.bin")

			if tt.content != nil {
				require.NoError(t, os.WriteFile(path, tt.content, 0o644))
			}

			err := VerifyFile(path, tt.expectedSHA1)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVerifyFile_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	// 1 MB of repeated bytes.
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i % 256)
	}
	require.NoError(t, os.WriteFile(path, data, 0o644))

	expected := sha1Hex(data)
	assert.NoError(t, VerifyFile(path, expected))
}
