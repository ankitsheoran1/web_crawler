package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"ankit/web_crawler/internal/config"
	"ankit/web_crawler/internal/repo"
	"ankit/web_crawler/internal/service"
)

// Handler serves HTTP API routes for crawl jobs.
type Handler struct {
	log   *zap.Logger
	cfg   config.Config
	crawl *service.Crawl
}

// NewHandler returns an API handler wired to the crawl service.
func NewHandler(log *zap.Logger, cfg config.Config, crawl *service.Crawl) *Handler {
	return &Handler{log: log, cfg: cfg, crawl: crawl}
}

// Register attaches routes to mux (Go 1.22+ patterns).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/crawl", h.submitCrawl)
	mux.HandleFunc("GET /v1/jobs/{id}", h.getJob)
}

func (h *Handler) submitCrawl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body submitCrawlBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.jsonError(w, http.StatusBadRequest, "invalid json")
		return
	}

	params := repo.CrawlParams{URL: strings.TrimSpace(body.URL)}
	if body.MaxDepth != nil {
		params.MaxDepth = *body.MaxDepth
	} else {
		params.MaxDepth = h.cfg.MaxDepth
	}
	if body.Workers != nil {
		params.Workers = *body.Workers
	} else {
		params.Workers = h.cfg.Workers
	}
	if body.URLCooldown != nil {
		d, err := time.ParseDuration(*body.URLCooldown)
		if err != nil {
			h.jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid url_cooldown: %v", err))
			return
		}
		if d < 0 {
			h.jsonError(w, http.StatusBadRequest, "url_cooldown must be non-negative")
			return
		}
		params.URLCooldown = repo.FlexibleDuration(d)
	} else {
		params.URLCooldown = repo.FlexibleDuration(h.cfg.URLCooldown)
	}

	if body.UseBrowser != nil {
		params.UseBrowser = *body.UseBrowser
	}
	if body.BrowserTimeout != nil {
		d, err := time.ParseDuration(*body.BrowserTimeout)
		if err != nil {
			h.jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid browser_timeout: %v", err))
			return
		}
		if d < 0 {
			h.jsonError(w, http.StatusBadRequest, "browser_timeout must be non-negative")
			return
		}
		params.BrowserTimeout = repo.FlexibleDuration(d)
	}

	if body.RequestTimeout != nil {
		d, err := time.ParseDuration(*body.RequestTimeout)
		if err != nil {
			h.jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid request_timeout: %v", err))
			return
		}
		if d < 0 {
			h.jsonError(w, http.StatusBadRequest, "request_timeout must be non-negative")
			return
		}
		params.RequestTimeout = repo.FlexibleDuration(d)
	}

	job, err := h.crawl.Submit(r.Context(), params)
	if err != nil {
		h.log.Warn("submit rejected", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	h.log.Debug("job submitted",
		zap.String("job_id", job.ID),
		zap.String("url", params.URL),
		zap.Int("max_depth", params.MaxDepth),
		zap.Int("workers", params.Workers),
		zap.Duration("url_cooldown", time.Duration(params.URLCooldown)),
		zap.Bool("use_browser", params.UseBrowser),
	)

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":     job.ID,
		"kind":   job.Kind,
		"status": job.Status,
	})
}

func (h *Handler) getJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := r.PathValue("id")
	job, err := h.crawl.Get(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			h.jsonError(w, http.StatusNotFound, "not found")
			return
		}
		h.jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	_ = json.NewEncoder(w).Encode(job)
}

func (h *Handler) jsonError(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type submitCrawlBody struct {
	URL         string  `json:"url"`
	MaxDepth    *int    `json:"max_depth"`
	Workers     *int    `json:"workers"`
	URLCooldown *string `json:"url_cooldown"`
	UseBrowser  *bool   `json:"use_browser"`
	BrowserTimeout *string `json:"browser_timeout"`
	RequestTimeout *string `json:"request_timeout"`
}
