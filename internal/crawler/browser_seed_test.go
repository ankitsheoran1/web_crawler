package crawler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ankit/web_crawler/internal/config"
	"ankit/web_crawler/internal/crawler"
	"ankit/web_crawler/internal/web"
)

type stubBrowser struct {
	calls int
	final string
	html  string
	err1  error
}

func (s stubBrowser) FetchRenderedHTML(ctx context.Context, url string) (string, string, error) {
	// Not used in this test.
	return s.final, s.html, nil
}

func (s *stubBrowser) FetchRenderedHTMLWithStrategy(ctx context.Context, url string, strategy web.NavStrategy) (string, string, error) {
	s.calls++
	if s.calls == 1 {
		return "", "", s.err1
	}
	return s.final, s.html, nil
}

func TestCrawl_seedBrowserExtractsMetadata(t *testing.T) {
	t.Parallel()
	b := &stubBrowser{
		final: "https://example.com/",
		html:  `<!doctype html><html><head><title>Hello</title><meta name="description" content="Desc here"></head><body><a href="/a">a</a>Body text</body></html>`,
		err1:  errors.New("page load error net::ERR_HTTP2_PROTOCOL_ERROR"),
	}
	cfg := config.Config{
		MaxDepth:            1,
		Workers:             1,
		URLCooldown:         0,
		HTTPTimeout:         2 * time.Second,
		SeedBrowser:         true,
		SeedBrowserTimeout:  2 * time.Second,
	}

	res, err := crawler.Crawl(context.Background(), nil, "https://example.com/", cfg, nil, b)
	if err != nil {
		t.Fatal(err)
	}
	if b.calls != 2 {
		t.Fatalf("expected 2 browser calls (retry), got %d", b.calls)
	}
	if res.Meta == nil || res.Meta.Title != "Hello" || res.Meta.Description != "Desc here" {
		t.Fatalf("meta=%#v", res.Meta)
	}
	if res.Seed == nil || res.Seed.FinalURL != "https://example.com/" {
		t.Fatalf("seed=%#v", res.Seed)
	}
	if len(res.URLs) < 1 {
		t.Fatalf("urls=%#v", res.URLs)
	}
}

