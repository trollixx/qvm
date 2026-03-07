package install

import (
	"crypto/sha1" //nolint:gosec // SHA1 is mandated by Qt's package repository protocol; cannot substitute
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// VerifyFile checks that the file at path has the expected SHA1 hex digest.
// If expectedSHA1 is empty, the check is skipped and nil is returned.
func VerifyFile(path, expectedSHA1 string) error {
	if expectedSHA1 == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s for verification: %w", path, err)
	}
	defer f.Close()

	h := sha1.New() //nolint:gosec // SHA1 is mandated by Qt's package repository protocol; cannot substitute
	_, err = io.Copy(h, f)
	if err != nil {
		return fmt.Errorf("hashing %s: %w", path, err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != expectedSHA1 {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", path, expectedSHA1, got)
	}
	return nil
}
