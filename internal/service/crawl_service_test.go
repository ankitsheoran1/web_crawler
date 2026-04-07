package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"ankit/web_crawler/internal/config"
	"ankit/web_crawler/internal/repo"
)

func TestSubmit_validationEmptyURL(t *testing.T) {
	t.Parallel()
	s := newTestService(t, config.Config{HTTPTimeout: time.Second, RequestTimeout: time.Minute}, nil)
	_, err := s.Submit(context.Background(), repo.CrawlParams{URL: "", Workers: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSubmit_validationWorkers(t *testing.T) {
	t.Parallel()
	s := newTestService(t, config.Config{HTTPTimeout: time.Second, RequestTimeout: time.Minute}, nil)
	_, err := s.Submit(context.Background(), repo.CrawlParams{URL: "https://example.com", Workers: 0})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSubmit_validationMaxDepth(t *testing.T) {
	t.Parallel()
	s := newTestService(t, config.Config{HTTPTimeout: time.Second, RequestTimeout: time.Minute}, nil)
	_, err := s.Submit(context.Background(), repo.CrawlParams{URL: "https://example.com", Workers: 1, MaxDepth: -1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSubmit_validationHostname(t *testing.T) {
	t.Parallel()
	s := newTestService(t, config.Config{HTTPTimeout: time.Second, RequestTimeout: time.Minute}, nil)
	_, err := s.Submit(context.Background(), repo.CrawlParams{URL: "/nope", Workers: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSubmit_runsToCompletion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
	}))
	t.Cleanup(ts.Close)

	cfg := config.Config{
		Workers:        2,
		URLCooldown:    0,
		HTTPTimeout:    5 * time.Second,
		RequestTimeout: 30 * time.Second,
	}
	s := newTestService(t, cfg, ts.Client())
	job, err := s.Submit(context.Background(), repo.CrawlParams{
		URL:      ts.URL + "/",
		MaxDepth: 0,
		Workers:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var final repo.Job
	for time.Now().Before(deadline) {
		final, err = s.Get(context.Background(), job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if final.Status == repo.JobCompleted || final.Status == repo.JobFailed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if final.Status != repo.JobCompleted {
		t.Fatalf("status=%s err=%q", final.Status, final.Error)
	}
	if final.Result == nil || final.Result.URLCount < 1 {
		t.Fatalf("result=%+v", final.Result)
	}
}

func TestSubmit_timeoutStillReturnsPartialResult(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body><a href="/x">x</a></body></html>`))
	}))
	t.Cleanup(ts.Close)

	cfg := config.Config{
		Workers:        2,
		URLCooldown:    0,
		HTTPTimeout:    5 * time.Second,
		RequestTimeout: 50 * time.Millisecond,
	}
	s := newTestService(t, cfg, ts.Client())
	job, err := s.Submit(context.Background(), repo.CrawlParams{
		URL:      ts.URL + "/",
		MaxDepth: 1,
		Workers:  2,
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var final repo.Job
	for time.Now().Before(deadline) {
		final, err = s.Get(context.Background(), job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if final.Status == repo.JobCompleted || final.Status == repo.JobFailed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if final.Status != repo.JobCompleted {
		t.Fatalf("status=%s err=%q", final.Status, final.Error)
	}
	if final.Result == nil {
		t.Fatalf("missing result (err=%q)", final.Error)
	}
	// We expect some error text because the context timed out.
	if final.Error == "" {
		t.Fatalf("expected job.error to be set for partial completion")
	}
}

func newTestService(t *testing.T, cfg config.Config, client *http.Client) *Crawl {
	t.Helper()
	return NewCrawl(zap.NewNop(), repo.NewStore(), cfg, client, nil)
}
