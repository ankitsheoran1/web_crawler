package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"ankit/web_crawler/internal/api"
	"ankit/web_crawler/internal/config"
	"ankit/web_crawler/internal/repo"
	"ankit/web_crawler/internal/service"
)

func TestSubmitCrawl_accepted(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>x</body></html>`))
	}))
	t.Cleanup(ts.Close)

	cfg := config.Config{
		MaxDepth:       0,
		Workers:        2,
		URLCooldown:    0,
		HTTPTimeout:    5 * time.Second,
		RequestTimeout: 30 * time.Second,
	}
	svc := service.NewCrawl(zap.NewNop(), repo.NewStore(), cfg, ts.Client(), nil)
	h := api.NewHandler(zap.NewNop(), cfg, svc)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"url":` + jsonString(ts.URL+`/`) + `}`
	req := httptest.NewRequest(http.MethodPost, "/v1/crawl", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		ID     string `json:"id"`
		Kind   string `json:"kind"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.ID == "" || out.Kind != "crawl" || out.Status != "pending" {
		t.Fatalf("%+v", out)
	}
}

func TestGetJob_notFound(t *testing.T) {
	t.Parallel()
	cfg := config.Config{HTTPTimeout: time.Second, RequestTimeout: time.Minute, Workers: 2}
	svc := service.NewCrawl(zap.NewNop(), repo.NewStore(), cfg, nil, nil)
	h := api.NewHandler(zap.NewNop(), cfg, svc)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/does-not-exist", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSubmitCrawl_invalidJSON(t *testing.T) {
	t.Parallel()
	cfg := config.Config{HTTPTimeout: time.Second, RequestTimeout: time.Minute, Workers: 2}
	svc := service.NewCrawl(zap.NewNop(), repo.NewStore(), cfg, nil, nil)
	h := api.NewHandler(zap.NewNop(), cfg, svc)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/v1/crawl", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("code=%d", rr.Code)
	}
}

func TestSubmitCrawl_invalidURLCooldown(t *testing.T) {
	t.Parallel()
	cfg := config.Config{HTTPTimeout: time.Second, RequestTimeout: time.Minute, Workers: 2}
	svc := service.NewCrawl(zap.NewNop(), repo.NewStore(), cfg, nil, nil)
	h := api.NewHandler(zap.NewNop(), cfg, svc)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"url":"https://example.com","url_cooldown":"bad"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/crawl", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
