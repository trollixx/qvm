package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestDiffDocs(t *testing.T) {
	tests := []struct {
		name      string
		requested []string
		installed []string
		want      []string
	}{
		{
			name:      "empty requested returns nil",
			requested: nil,
			installed: []string{"a"},
			want:      nil,
		},
		{
			name:      "installed has wildcard returns nil",
			requested: []string{"a", "b"},
			installed: []string{"*"},
			want:      nil,
		},
		{
			name:      "requested wildcard passes through",
			requested: []string{"*"},
			installed: []string{"a"},
			want:      []string{"*"},
		},
		{
			name:      "both wildcard returns nil",
			requested: []string{"*"},
			installed: []string{"*"},
			want:      nil,
		},
		{
			name:      "normal diff",
			requested: []string{"a", "b", "c"},
			installed: []string{"a"},
			want:      []string{"b", "c"},
		},
		{
			name:      "empty installed",
			requested: []string{"a"},
			installed: nil,
			want:      []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffDocs(tt.requested, tt.installed)
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

func TestMergeDocs(t *testing.T) {
	tests := []struct {
		name      string
		existing  []string
		additions []string
		want      []string
	}{
		{
			name:      "existing has wildcard",
			existing:  []string{"*"},
			additions: []string{"a", "b"},
			want:      []string{"*"},
		},
		{
			name:      "additions has wildcard",
			existing:  []string{"a", "b"},
			additions: []string{"*"},
			want:      []string{"*"},
		},
		{
			name:      "both have wildcard",
			existing:  []string{"*"},
			additions: []string{"*"},
			want:      []string{"*"},
		},
		{
			name:      "normal merge",
			existing:  []string{"a"},
			additions: []string{"b"},
			want:      []string{"a", "b"},
		},
		{
			name:      "both empty",
			existing:  nil,
			additions: nil,
			want:      []string{},
		},
		{
			name:      "no wildcard with overlap",
			existing:  []string{"a", "b"},
			additions: []string{"b", "c"},
			want:      []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeDocs(tt.existing, tt.additions)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSendProgress(t *testing.T) {
	t.Run("nil channel does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			sendProgress(nil, ProgressEvent{Phase: "testing"})
		})
	})

	t.Run("full channel does not block", func(t *testing.T) {
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
		assert.Equal(t, float64(50), received.Percent)
	})
}
