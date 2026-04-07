package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestURLThrottle_waitCancel(t *testing.T) {
	t.Parallel()
	clock := time.Unix(0, 0).UTC()
	th := &urlThrottle{
		last:  make(map[string]time.Time),
		cool:  time.Second,
		clock: func() time.Time { return clock },
	}
	th.last["u"] = clock
	clock = clock.Add(500 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := th.wait(ctx, "u"); err != context.Canceled {
		t.Fatalf("err = %v", err)
	}
}

func TestURLThrottle_noCooldown(t *testing.T) {
	t.Parallel()
	th := newURLThrottle(0)
	if err := th.wait(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	th.markFetched("x")
}

func TestExtractLinks_sameHost(t *testing.T) {
	t.Parallel()
	page := "https://example.com/dir/page"
	root := "example.com"
	htmlDoc := `<!doctype html><a href="/a">x</a><a href="https://other.com/b">o</a><a href="https://example.com/c#f">c</a>`
	links, err := extractLinks(strings.NewReader(htmlDoc), page, root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"https://example.com/a": true,
		"https://example.com/c": true,
	}
	if len(links) != len(want) {
		t.Fatalf("links %#v", links)
	}
	for _, u := range links {
		if !want[u] {
			t.Fatalf("unexpected %q", u)
		}
	}
}

func TestExtractLinks_mailtoSkipped(t *testing.T) {
	t.Parallel()
	htmlDoc := `<a href="mailto:a@b.com">m</a>`
	links, err := extractLinks(strings.NewReader(htmlDoc), "https://example.com/", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("got %#v", links)
	}
}

func TestFetchAndExtract_non2xxIncludesBodySnippet(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("blocked by bot protection"))
	}))
	t.Cleanup(ts.Close)

	client := ts.Client()
	_, details, _, err := fetchAndExtract(context.Background(), client, ts.URL, ts.URL, "example.com", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if details == nil || !strings.Contains(details.BodySnippet, "blocked by bot protection") {
		t.Fatalf("expected body snippet in seed details, got %#v", details)
	}
	if strings.Contains(err.Error(), "body=") {
		t.Fatalf("did not expect body snippet in error, got %v", err)
	}
}
