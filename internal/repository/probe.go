package repository

import (
	"context"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

// ProbeResult holds the outcome of probing a single mirror.
type ProbeResult struct {
	URL       string
	Latency   time.Duration
	Reachable bool
}

// ProbeURLs measures response latency to each base URL concurrently.
// Each URL is probed by fetching the platform-specific desktop directory listing.
// Returns results sorted fastest-first; unreachable mirrors are appended last.
func ProbeURLs(ctx context.Context, urls []string, timeoutSeconds int) []ProbeResult {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	host := platformHost()

	results := make([]ProbeResult, len(urls))
	var wg sync.WaitGroup
	for i, base := range urls {
		wg.Add(1)
		i, base := i, base
		go func() {
			defer wg.Done()
			results[i] = probeOne(ctx, client, base, host)
		}()
	}
	wg.Wait()

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Reachable != results[j].Reachable {
			return results[i].Reachable
		}
		return results[i].Latency < results[j].Latency
	})
	return results
}

func probeOne(ctx context.Context, client *http.Client, base, host string) ProbeResult {
	probeURL := base + "online/qtsdkrepository/" + host + "/desktop/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return ProbeResult{URL: base}
	}
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return ProbeResult{URL: base}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	if resp.StatusCode >= 400 {
		return ProbeResult{URL: base}
	}
	return ProbeResult{URL: base, Latency: latency, Reachable: true}
}
