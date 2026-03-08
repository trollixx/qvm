package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQvmError_Error(t *testing.T) {
	tests := []struct {
		name        string
		err         *QvmError
		wantContain string
		wantExact   string
	}{
		{
			name:      "message only",
			err:       New(CodeUnknownVersion, "version not found"),
			wantExact: "version not found",
		},
		{
			name: "message with one suggestion",
			err: &QvmError{
				Code:        CodeUnknownVersion,
				Message:     "version 9.9.9 not found",
				Suggestions: []string{"try qt@6.7.0"},
			},
			wantExact: "version 9.9.9 not found\n\nSuggestions:\n  try qt@6.7.0",
		},
		{
			name: "message with multiple suggestions",
			err: &QvmError{
				Code:        CodeUnknownModule,
				Message:     "module not found",
				Suggestions: []string{"did you mean qtcharts?", "run qvm list-modules"},
			},
			wantExact: "module not found\n\nSuggestions:\n  did you mean qtcharts?\n  run qvm list-modules",
		},
		{
			name:      "empty message no suggestions",
			err:       New(CodeIOError, ""),
			wantExact: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if tt.wantExact != "" || tt.err.Message == "" {
				assert.Equal(t, tt.wantExact, got)
			}
			if tt.wantContain != "" {
				assert.Contains(t, got, tt.wantContain)
			}
		})
	}
}

func TestNew(t *testing.T) {
	err := New(CodeNetworkError, "connection refused")
	assert.Equal(t, CodeNetworkError, err.Code)
	assert.Equal(t, "connection refused", err.Message)
	assert.Nil(t, err.Suggestions)
	assert.NoError(t, err.Cause)
}

func TestNewf(t *testing.T) {
	err := Newf(CodeChecksumError, "expected %s got %s", "abc", "def")
	assert.Equal(t, CodeChecksumError, err.Code)
	assert.Equal(t, "expected abc got def", err.Message)
}

func TestWrap(t *testing.T) {
	cause := errors.New("underlying IO error")
	err := Wrap(CodeIOError, cause, "failed to read file")
	assert.Equal(t, CodeIOError, err.Code)
	assert.Equal(t, "failed to read file", err.Message)
	assert.Equal(t, cause, err.Cause)
}

func TestQvmError_Unwrap(t *testing.T) {
	t.Run("with cause", func(t *testing.T) {
		cause := errors.New("root cause")
		err := Wrap(CodeNetworkError, cause, "wrapper")
		assert.Equal(t, cause, err.Unwrap())
		// Also works with errors.Is / errors.Unwrap
		assert.ErrorIs(t, err, cause)
	})

	t.Run("without cause", func(t *testing.T) {
		err := New(CodeUnknownTool, "no cause")
		assert.NoError(t, err.Unwrap())
	})
}

func TestWithSuggestions(t *testing.T) {
	t.Run("adds suggestions to copy", func(t *testing.T) {
		original := New(CodeUnknownVersion, "not found")
		withSug := original.WithSuggestions("try A", "try B")

		assert.Empty(t, original.Suggestions, "original must remain unchanged")
		require.Len(t, withSug.Suggestions, 2)
		assert.Equal(t, []string{"try A", "try B"}, withSug.Suggestions)
	})

	t.Run("appends to existing suggestions", func(t *testing.T) {
		first := New(CodeUnknownVersion, "not found").WithSuggestions("try A")
		second := first.WithSuggestions("try B")

		assert.Equal(t, []string{"try A"}, first.Suggestions)
		assert.Equal(t, []string{"try A", "try B"}, second.Suggestions)
	})

	t.Run("deep copy prevents aliasing", func(t *testing.T) {
		original := New(CodeUnknownVersion, "not found").WithSuggestions("A")
		derived := original.WithSuggestions("B")

		// Mutate derived and verify original is not affected.
		derived.Suggestions[0] = "MUTATED"
		assert.Equal(t, "A", original.Suggestions[0],
			"mutation of derived must not affect original (deep copy)")
	})

	t.Run("preserves other fields", func(t *testing.T) {
		cause := errors.New("cause")
		original := Wrap(CodeIOError, cause, "msg")
		derived := original.WithSuggestions("hint")
		assert.Equal(t, CodeIOError, derived.Code)
		assert.Equal(t, "msg", derived.Message)
		assert.Equal(t, cause, derived.Cause)
	})
}

func TestIsQvmError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "QvmError from New",
			err:  New(CodeUnknownVersion, "test"),
			want: true,
		},
		{
			name: "QvmError from Wrap",
			err:  Wrap(CodeIOError, errors.New("io"), "msg"),
			want: true,
		},
		{
			name: "plain error",
			err:  errors.New("not a qvm error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "wrapped QvmError is detected via errors.As",
			err:  fmt.Errorf("wrapped: %w", New(CodeIOError, "inner")),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsQvmError(tt.err))
		})
	}
}
