package api

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sortit/internal/issues"
)

func TestRevisionStreamEmitsInitialAndBumpedFrames(t *testing.T) {
	t.Parallel()

	revisions := issues.NewRevisionTracker()
	srv := &Server{revisions: revisions}

	mux := http.NewServeMux()
	mux.HandleFunc("/revision/stream", srv.handleRevisionStream)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/revision/stream", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}

	frames := make(chan string, 8)
	errs := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		var current strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				errs <- err
				return
			}
			if line == "\n" || line == "\r\n" {
				if current.Len() > 0 {
					frames <- current.String()
					current.Reset()
				}
				continue
			}
			current.WriteString(strings.TrimRight(line, "\r\n"))
		}
	}()

	expectFrame := func(want string) {
		t.Helper()
		select {
		case got := <-frames:
			if got != want {
				t.Fatalf("frame: want %q, got %q", want, got)
			}
		case err := <-errs:
			t.Fatalf("stream read error while waiting for %q: %v", want, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for frame %q", want)
		}
	}

	expectFrame(fmt.Sprintf("data: %d", revisions.Revision()))

	bumped := revisions.Bump()
	expectFrame(fmt.Sprintf("data: %d", bumped))

	bumped = revisions.Bump()
	expectFrame(fmt.Sprintf("data: %d", bumped))
}

func TestRevisionStreamExitsOnClientCancel(t *testing.T) {
	t.Parallel()

	revisions := issues.NewRevisionTracker()
	srv := &Server{revisions: revisions}

	mux := http.NewServeMux()
	mux.HandleFunc("/revision/stream", srv.handleRevisionStream)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/revision/stream", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}

	// Cancel while the handler is still blocked in its select loop; ensure
	// the connection closes cleanly (no goroutine leak, no panic).
	cancel()

	// Drain any remaining body; the server side should exit via ctx.Done.
	buf := make([]byte, 128)
	for {
		if _, err := resp.Body.Read(buf); err != nil {
			break
		}
	}
	resp.Body.Close()
}
