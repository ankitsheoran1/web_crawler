package repo

import (
	"encoding/json"
	"time"
)

// JobKind identifies the async operation type returned to clients.
type JobKind string

const (
	JobKindCrawl JobKind = "crawl"
)

// JobStatus is the lifecycle of an async job.
type JobStatus string

const (
	JobPending    JobStatus = "pending"
	JobRunning    JobStatus = "running"
	JobCompleted  JobStatus = "completed"
	JobFailed     JobStatus = "failed"
	JobCancelled  JobStatus = "cancelled"
)

// FlexibleDuration marshals as a Go duration string (e.g. "500ms") in JSON.
type FlexibleDuration time.Duration

func (d FlexibleDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *FlexibleDuration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		dd, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		*d = FlexibleDuration(dd)
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*d = FlexibleDuration(n)
	return nil
}

// CrawlParams captures per-request crawl settings.
type CrawlParams struct {
	URL         string           `json:"url"`
	MaxDepth    int              `json:"max_depth"`
	Workers     int              `json:"workers"`
	URLCooldown FlexibleDuration `json:"url_cooldown"`
	UseBrowser  bool             `json:"use_browser,omitempty"`
	// BrowserTimeout applies only when UseBrowser is true for seed URL fetching.
	BrowserTimeout FlexibleDuration `json:"browser_timeout,omitempty"`
	// RequestTimeout limits total wall time for the whole job (seed + crawling).
	RequestTimeout FlexibleDuration `json:"request_timeout,omitempty"`
}

// FetchDetails captures useful diagnostics about fetching a single URL.
type FetchDetails struct {
	RequestedURL string `json:"requested_url"`
	FinalURL     string `json:"final_url,omitempty"`
	StatusCode   int    `json:"status_code,omitempty"`
	Status       string `json:"status,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	DurationMs   int64  `json:"duration_ms,omitempty"`
	Skipped      bool   `json:"skipped,omitempty"` // e.g., non-HTML content
	Error        string `json:"error,omitempty"`
	BodySnippet  string `json:"body_snippet,omitempty"`
}

// HTMLMetadata captures lightweight metadata extracted from a document.
type HTMLMetadata struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	BodySnippet string `json:"body_snippet,omitempty"`
}

// CrawlResult is persisted when a crawl finishes.
type CrawlResult struct {
	URLCount     int           `json:"url_count"`
	FailureCount int           `json:"failure_count"`
	// Errors is a small set of representative, human-friendly error messages (typically 1-2).
	Errors []string `json:"errors"`
	// ErrorBreakdown groups failures into classes (e.g. "http_403", "context_deadline_exceeded").
	ErrorBreakdown map[string]int `json:"error_breakdown,omitempty"`
	SeedFetch    *FetchDetails `json:"seed_fetch,omitempty"`
	SeedMeta     *HTMLMetadata `json:"seed_meta,omitempty"`
}

// Job is the durable record for a submitted async operation.
type Job struct {
	ID        string    `json:"id"`
	Kind      JobKind   `json:"kind"`
	Status    JobStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	Params CrawlParams `json:"params"`
	Result *CrawlResult `json:"result,omitempty"`
	Error  string       `json:"error,omitempty"`
}

func NewCrawlJob(id string, params CrawlParams) Job {
	return Job{
		ID:        id,
		Kind:      JobKindCrawl,
		Status:    JobPending,
		CreatedAt: time.Now().UTC(),
		Params:    params,
	}
}
