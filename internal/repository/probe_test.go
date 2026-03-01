package repository_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trollixx/qvm/internal/repository"
)

func okHandler(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func TestProbeURLs_Reachable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(okHandler))
	defer ts.Close()

	results := repository.ProbeURLs(context.Background(), []string{ts.URL + "/"}, 5)

	require.Len(t, results, 1)
	assert.Equal(t, ts.URL+"/", results[0].URL)
	assert.True(t, results[0].Reachable)
	assert.Greater(t, results[0].Latency, time.Duration(0))
}

func TestProbeURLs_Unreachable(t *testing.T) {
	// Start a server, capture its URL, then close it so connections are refused.
	ts := httptest.NewServer(http.HandlerFunc(okHandler))
	badURL := ts.URL + "/"
	ts.Close()

	results := repository.ProbeURLs(context.Background(), []string{badURL}, 5)

	require.Len(t, results, 1)
	assert.Equal(t, badURL, results[0].URL)
	assert.False(t, results[0].Reachable)
}

func TestProbeURLs_HTTP404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	results := repository.ProbeURLs(context.Background(), []string{ts.URL + "/"}, 5)

	require.Len(t, results, 1)
	assert.False(t, results[0].Reachable)
}

func TestProbeURLs_ReachableBeforeUnreachable(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(okHandler))
	defer good.Close()

	// Closed server → connection refused.
	bad := httptest.NewServer(http.HandlerFunc(okHandler))
	badURL := bad.URL + "/"
	bad.Close()

	// Pass bad URL first to verify sorting places reachable results first.
	results := repository.ProbeURLs(context.Background(), []string{badURL, good.URL + "/"}, 5)

	require.Len(t, results, 2)
	assert.True(t, results[0].Reachable, "first result should be reachable")
	assert.False(t, results[1].Reachable, "second result should be unreachable")
}

func TestProbeURLs_Empty(t *testing.T) {
	results := repository.ProbeURLs(context.Background(), nil, 5)
	assert.Empty(t, results)
}

func TestProbeURLs_PreservesURL(t *testing.T) {
	// Verify the URL field in the result matches the input base URL, not the probe sub-path.
	ts := httptest.NewServer(http.HandlerFunc(okHandler))
	defer ts.Close()

	base := ts.URL + "/"
	results := repository.ProbeURLs(context.Background(), []string{base}, 5)

	require.Len(t, results, 1)
	assert.Equal(t, base, results[0].URL)
}
