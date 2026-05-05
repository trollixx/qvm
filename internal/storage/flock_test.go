package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLockFile_RemovesLockFileOnUnlock(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "registry.json")
	lockPath := target + ".lock"

	unlock, err := LockFile(target)
	if err != nil {
		t.Fatalf("LockFile: %v", err)
	}
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Fatalf("lock file should exist while held: %v", statErr)
	}
	unlock()
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Fatalf("lock file should be removed after unlock, got err=%v", statErr)
	}
}

func TestLockFile_SerializesConcurrentHolders(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "registry.json")

	first, err := LockFile(target)
	if err != nil {
		t.Fatalf("first LockFile: %v", err)
	}

	acquired := make(chan struct{})
	var second func()
	var wg sync.WaitGroup
	wg.Go(func() {
		s, lockErr := LockFile(target)
		if lockErr != nil {
			t.Errorf("second LockFile: %v", lockErr)
			return
		}
		second = s
		close(acquired)
	})

	select {
	case <-acquired:
		t.Fatalf("second LockFile acquired while first still held")
	case <-time.After(100 * time.Millisecond):
	}

	first()

	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatalf("second LockFile did not acquire after first released")
	}

	wg.Wait()
	second()
}
