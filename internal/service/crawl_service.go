package service

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"ankit/web_crawler/internal/config"
	"ankit/web_crawler/internal/crawler"
	"ankit/web_crawler/internal/repo"
	"ankit/web_crawler/internal/web"
)

// Crawl submits an async crawl and returns identifiers for polling.
type Crawl struct {
	log    *zap.Logger
	store  *repo.Store
	cfg    config.Config
	client *http.Client
	browser web.BrowserFetcher
}

var httpStatusRe = regexp.MustCompile(`\bstatus\s+(\d{3})\b`)

func NewCrawl(log *zap.Logger, store *repo.Store, cfg config.Config, client *http.Client, browser web.BrowserFetcher) *Crawl {
	if client == nil {
		client = web.NewHTTPClient(cfg.HTTPTimeout)
	}
	return &Crawl{log: log, store: store, cfg: cfg, client: client, browser: browser}
}

// Submit validates input, persists a pending job, and starts processing in the background.
func (s *Crawl) Submit(ctx context.Context, params repo.CrawlParams) (repo.Job, error) {
	if params.URL == "" {
		return repo.Job{}, fmt.Errorf("url is required")
	}
	if _, err := web.Hostname(params.URL); err != nil {
		return repo.Job{}, err
	}
	if params.Workers < 1 {
		return repo.Job{}, fmt.Errorf("workers must be >= 1")
	}
	if params.MaxDepth < 0 {
		return repo.Job{}, fmt.Errorf("max_depth must be >= 0")
	}

	id := uuid.NewString()
	job := repo.NewCrawlJob(id, params)
	s.store.Save(job)

	go s.runJob(context.Background(), id)

	return job, nil
}

func (s *Crawl) Get(_ context.Context, id string) (repo.Job, error) {
	return s.store.Get(id)
}

func (s *Crawl) runJob(bg context.Context, id string) {
	timeout := s.cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	// Allow per-job override (from POST body).
	job, err := s.store.Get(id)
	if err != nil {
		return
	}
	if rt := time.Duration(job.Params.RequestTimeout); rt > 0 {
		timeout = rt
	}

	ctx, cancel := context.WithTimeout(bg, timeout)
	defer cancel()

	s.log.Debug("job starting", zap.String("job_id", id), zap.Duration("timeout", timeout))

	_ = s.store.Update(id, func(j *repo.Job) error {
		j.Status = repo.JobRunning
		now := time.Now().UTC()
		j.StartedAt = &now
		return nil
	})

	p := job.Params
	s.log.Debug("job params",
		zap.String("job_id", id),
		zap.String("url", p.URL),
		zap.Int("max_depth", p.MaxDepth),
		zap.Int("workers", p.Workers),
		zap.Duration("url_cooldown", time.Duration(p.URLCooldown)),
	)
	crawlCfg := config.Config{
		MaxDepth:    p.MaxDepth,
		Workers:     p.Workers,
		URLCooldown: time.Duration(p.URLCooldown),
		HTTPTimeout: s.cfg.HTTPTimeout,
		SeedBrowser: p.UseBrowser,
	}
	if p.UseBrowser {
		bt := time.Duration(p.BrowserTimeout)
		if bt <= 0 {
			bt = 45 * time.Second
		}
		crawlCfg.SeedBrowserTimeout = bt
	}

	res, err := crawler.Crawl(ctx, s.client, p.URL, crawlCfg, s.log, s.browser)
	finished := time.Now().UTC()

	if err != nil {
		_ = s.store.Update(id, func(j *repo.Job) error {
			j.Status = repo.JobFailed
			j.FinishedAt = &finished
			j.Error = err.Error()
			return nil
		})
		s.log.Warn("crawl failed", zap.String("job_id", id), zap.Error(err))
		return
	}

	if ctx.Err() != nil {
		// Return partial progress instead of losing work.
		ctxErr := ctx.Err()
		samples, breakdown := summarizeErrors(res.Errors, 2)
		_ = s.store.Update(id, func(j *repo.Job) error {
			j.Status = repo.JobCompleted
			j.FinishedAt = &finished
			j.Result = &repo.CrawlResult{
				URLCount:     len(res.URLs),
				FailureCount: len(res.Errors),
				Errors:       samples,
				ErrorBreakdown: breakdown,
				SeedFetch:    res.Seed,
				SeedMeta:     res.Meta,
			}
			j.Error = ctxErr.Error()
			return nil
		})
		s.log.Warn("job completed with context error (partial result)",
			zap.String("job_id", id),
			zap.Error(ctxErr),
			zap.Int("urls", len(res.URLs)),
			zap.Int("errors", len(res.Errors)),
		)
		return
	}

	samples, breakdown := summarizeErrors(res.Errors, 2)
	_ = s.store.Update(id, func(j *repo.Job) error {
		j.Status = repo.JobCompleted
		j.FinishedAt = &finished
		j.Result = &repo.CrawlResult{
			URLCount:     len(res.URLs),
			FailureCount: len(res.Errors),
			Errors:       samples,
			ErrorBreakdown: breakdown,
			SeedFetch:    res.Seed,
			SeedMeta:     res.Meta,
		}
		return nil
	})
	s.log.Info("crawl completed", zap.String("job_id", id), zap.Int("urls", len(res.URLs)))
}

func summarizeErrors(errs []string, maxSamples int) ([]string, map[string]int) {
	breakdown := make(map[string]int)
	if len(errs) == 0 {
		return nil, breakdown
	}

	// Keep first-seen sample per class.
	sampleByClass := make(map[string]string)
	for _, e := range errs {
		class, sample := classifyError(e)
		breakdown[class]++
		if _, ok := sampleByClass[class]; !ok && sample != "" {
			sampleByClass[class] = sample
		}
	}

	// Prefer classes with highest counts.
	type kv struct {
		k string
		v int
	}
	var classes []kv
	for k, v := range breakdown {
		classes = append(classes, kv{k: k, v: v})
	}
	sort.Slice(classes, func(i, j int) bool {
		if classes[i].v == classes[j].v {
			return classes[i].k < classes[j].k
		}
		return classes[i].v > classes[j].v
	})

	var samples []string
	for _, c := range classes {
		if len(samples) >= maxSamples {
			break
		}
		if s, ok := sampleByClass[c.k]; ok && s != "" {
			samples = append(samples, s)
		}
	}
	return samples, breakdown
}

func classifyError(e string) (class string, sample string) {
	le := strings.ToLower(e)
	switch {
	case strings.Contains(le, "context deadline exceeded"):
		return "context_deadline_exceeded", "context deadline exceeded"
	case strings.Contains(le, "context canceled"):
		return "context_canceled", "context canceled"
	}

	if m := httpStatusRe.FindStringSubmatch(le); len(m) == 2 {
		code, _ := strconv.Atoi(m[1])
		switch {
		case code == 403:
			return "http_403", "403 forbidden"
		case code >= 400 && code < 500:
			return "http_4xx", "http 4xx"
		case code >= 500 && code < 600:
			return "http_5xx", "http 5xx"
		default:
			return "http_other", "http error"
		}
	}

	switch {
	case strings.Contains(le, "no such host"):
		return "dns", "dns lookup failed"
	case strings.Contains(le, "tls"):
		return "tls", "tls handshake failed"
	case strings.Contains(le, "connection refused"):
		return "connection_refused", "connection refused"
	case strings.Contains(le, "i/o timeout"):
		return "io_timeout", "i/o timeout"
	}

	return "other", "other error"
}
