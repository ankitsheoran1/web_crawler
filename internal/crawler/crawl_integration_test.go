package crawler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"ankit/web_crawler/internal/config"
	"ankit/web_crawler/internal/crawler"
)

func TestCrawl_followsInternalLinks(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><a href="/b">b</a></body></html>`))
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><a href="/">home</a></body></html>`))
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := ts.Client()
	cfg := config.Config{MaxDepth: 2, Workers: 2, URLCooldown: 0, HTTPTimeout: 5 * time.Second}
	res, err := crawler.Crawl(context.Background(), client, ts.URL+"/", cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.URLs) < 2 {
		t.Fatalf("urls %#v", res.URLs)
	}
	if res.Seed == nil || res.Seed.RequestedURL == "" {
		t.Fatalf("missing seed details: %#v", res.Seed)
	}
	joined := strings.Join(res.URLs, "\n")
	if !strings.Contains(joined, "/b") {
		t.Fatalf("missing /b in %#v", res.URLs)
	}
}

func TestCrawl_depthZeroOnlyRoot(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><a href="/inner">in</a></body></html>`))
	})
	mux.HandleFunc("/inner", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>in</body></html>`))
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := config.Config{MaxDepth: 0, Workers: 1, URLCooldown: 0, HTTPTimeout: 3 * time.Second}
	res, err := crawler.Crawl(context.Background(), ts.Client(), ts.URL+"/", cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.URLs) != 1 {
		t.Fatalf("want 1 url, got %#v", res.URLs)
	}
	if res.Seed == nil || res.Seed.StatusCode != 200 {
		t.Fatalf("seed=%#v", res.Seed)
	}
}

func TestCrawl_invalidStartURLHost(t *testing.T) {
	t.Parallel()
	cfg := config.Config{MaxDepth: 1, Workers: 1, URLCooldown: 0, HTTPTimeout: time.Second}
	_, err := crawler.Crawl(context.Background(), nil, "/nope", cfg, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCrawl_nonHTMLSkipped(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(ts.Close)

	cfg := config.Config{MaxDepth: 1, Workers: 1, URLCooldown: 0, HTTPTimeout: 3 * time.Second}
	res, err := crawler.Crawl(context.Background(), ts.Client(), ts.URL+"/", cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.URLs) != 1 {
		t.Fatalf("urls %#v", res.URLs)
	}
	if res.Seed == nil || !res.Seed.Skipped {
		t.Fatalf("seed=%#v", res.Seed)
	}
}

func TestCrawl_HTTPErrorRecorded(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	t.Cleanup(ts.Close)

	cfg := config.Config{MaxDepth: 0, Workers: 1, URLCooldown: 0, HTTPTimeout: 3 * time.Second}
	res, err := crawler.Crawl(context.Background(), ts.Client(), ts.URL+"/", cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) < 1 {
		t.Fatalf("errors %#v", res.Errors)
	}
}

func TestCrawl_workersZeroBecomesOne(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	}))
	t.Cleanup(ts.Close)

	cfg := config.Config{MaxDepth: 0, Workers: 0, URLCooldown: 0, HTTPTimeout: 3 * time.Second}
	_, err := crawler.Crawl(context.Background(), ts.Client(), ts.URL+"/", cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCrawl_urlCapStopsAt150(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var sb strings.Builder
		sb.WriteString("<html><body>")
		for i := 0; i < 300; i++ {
			sb.WriteString(`<a href="/p`)
			sb.WriteString(strconv.Itoa(i))
			sb.WriteString(`">x</a>`)
		}
		sb.WriteString("</body></html>")
		_, _ = w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/p", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	})
	// Register handlers for /p0..../p299
	for i := 0; i < 300; i++ {
		path := "/p" + strconv.Itoa(i)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
		})
	}
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := config.Config{MaxDepth: 2, Workers: 20, URLCooldown: 0, HTTPTimeout: 5 * time.Second}
	res, err := crawler.Crawl(context.Background(), ts.Client(), ts.URL+"/", cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.URLs) != 150 {
		t.Fatalf("want 150 urls, got %d", len(res.URLs))
	}
}

func TestCrawl_seedRedirectChangesRootHost(t *testing.T) {
	t.Parallel()

	// Target server (final host) with an internal link.
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><a href="/b">b</a></body></html>`))
	})
	mux2.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	})
	ts2 := httptest.NewServer(mux2)
	t.Cleanup(ts2.Close)

	// Seed server that redirects to the target server (different host).
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ts2.URL+"/", http.StatusFound)
	}))
	t.Cleanup(ts1.Close)

	cfg := config.Config{MaxDepth: 1, Workers: 2, URLCooldown: 0, HTTPTimeout: 5 * time.Second}
	res, err := crawler.Crawl(context.Background(), ts1.Client(), ts1.URL+"/", cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(res.URLs, "\n")
	if !strings.Contains(joined, ts2.URL+"/b") {
		t.Fatalf("expected to follow /b on redirected host; urls=%#v", res.URLs)
	}
}
