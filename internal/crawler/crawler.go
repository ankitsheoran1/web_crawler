package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"go.uber.org/zap"
	"golang.org/x/net/html"

	"ankit/web_crawler/internal/config"
	"ankit/web_crawler/internal/repo"
	"ankit/web_crawler/internal/web"
)

// Result is produced when a crawl finishes (success or partial).
type Result struct {
	URLs   []string
	Errors []string
	Seed   *repo.FetchDetails
	Meta   *repo.HTMLMetadata
}

type urlThrottle struct {
	mu    sync.Mutex
	last  map[string]time.Time
	cool  time.Duration
	clock func() time.Time
}

func newURLThrottle(cool time.Duration) *urlThrottle {
	return &urlThrottle{
		last:  make(map[string]time.Time),
		cool:  cool,
		clock: time.Now,
	}
}

func (t *urlThrottle) wait(ctx context.Context, key string) error {
	if t.cool <= 0 {
		return nil
	}
	for {
		var sleep time.Duration
		now := t.clock().UTC()
		t.mu.Lock()
		if last, ok := t.last[key]; ok {
			next := last.Add(t.cool)
			if now.Before(next) {
				sleep = next.Sub(now)
			}
		}
		t.mu.Unlock()

		if sleep > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleep):
			}
			continue
		}
		return nil
	}
}

func (t *urlThrottle) markFetched(key string) {
	if t.cool <= 0 {
		return
	}
	t.mu.Lock()
	t.last[key] = t.clock().UTC()
	t.mu.Unlock()
}

// Crawl fetches HTML on the same host as startURL, follows links up to cfg.MaxDepth,
// and limits concurrent fetches to cfg.Workers. The same normalized URL is not
// requested again until cfg.URLCooldown has elapsed (enforced per fetch).
func Crawl(ctx context.Context, client *http.Client, startURL string, cfg config.Config, log *zap.Logger, browser web.BrowserFetcher) (Result, error) {
	if log == nil {
		log = zap.NewNop()
	}
	log = log.Named("crawler")

	if client == nil {
		client = web.NewHTTPClient(cfg.HTTPTimeout)
	}
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}

	rootHost, err := web.Hostname(startURL)
	if err != nil {
		return Result{}, err
	}

	log.Debug("crawl starting",
		zap.String("start_url", startURL),
		zap.String("root_host", rootHost),
		zap.Int("max_depth", cfg.MaxDepth),
		zap.Int("workers", cfg.Workers),
		zap.Duration("url_cooldown", cfg.URLCooldown),
	)

	sem := make(chan struct{}, cfg.Workers)
	throttle := newURLThrottle(cfg.URLCooldown)
	var wg sync.WaitGroup

	const maxUniqueURLs = 150

	var visitMu sync.Mutex
	seen := make(map[string]struct{})

	var outMu sync.Mutex
	var urls []string
	var errs []string
	var seed *repo.FetchDetails
	var meta *repo.HTMLMetadata

	var crawl func(string, int)
	crawl = func(raw string, depth int) {
		defer wg.Done()

		if ctx.Err() != nil {
			return
		}
		if depth > cfg.MaxDepth {
			return
		}

		norm, err := web.Normalize(raw)
		if err != nil {
			outMu.Lock()
			errs = append(errs, fmt.Sprintf("normalize %s: %v", raw, err))
			outMu.Unlock()
			return
		}

		visitMu.Lock()
		if len(seen) >= maxUniqueURLs {
			visitMu.Unlock()
			log.Debug("url cap reached, skipping scheduling", zap.Int("cap", maxUniqueURLs), zap.String("url", norm))
			return
		}
		if _, ok := seen[norm]; ok {
			visitMu.Unlock()
			return
		}
		seen[norm] = struct{}{}
		visitMu.Unlock()

		outMu.Lock()
		urls = append(urls, norm)
		outMu.Unlock()

		log.Debug("scheduled url", zap.String("url", norm), zap.Int("depth", depth))

		if err := throttle.wait(ctx, norm); err != nil {
			log.Debug("throttle wait aborted", zap.String("url", norm), zap.Error(err))
			return
		}

		select {
		case <-ctx.Done():
			return
		case sem <- struct{}{}:
		}

		start := time.Now().UTC()
		var links []string
		var details *repo.FetchDetails
		var m *repo.HTMLMetadata
		var ferr error

		if depth == 0 && cfg.SeedBrowser && browser != nil {
			links, details, m, ferr = fetchAndExtractWithBrowser(ctx, browser, norm, raw, rootHost, cfg.SeedBrowserTimeout)
		} else {
			links, details, m, ferr = fetchAndExtract(ctx, client, norm, raw, rootHost, log)
		}
		throttle.markFetched(norm)
		<-sem

		if depth == 0 {
			outMu.Lock()
			seed = details
			meta = m
			outMu.Unlock()
		}

		if ferr != nil {
			outMu.Lock()
			errs = append(errs, ferr.Error())
			outMu.Unlock()
			log.Debug("fetch failed",
				zap.String("url", norm),
				zap.Int("depth", depth),
				zap.Duration("elapsed", time.Since(start)),
				zap.Error(ferr),
			)
		} else {
			log.Debug("fetch ok",
				zap.String("url", norm),
				zap.Int("depth", depth),
				zap.Int("links_found", len(links)),
				zap.Duration("elapsed", time.Since(start)),
			)
		}

		for _, link := range links {
			if depth >= cfg.MaxDepth {
				break
			}
			visitMu.Lock()
			over := len(seen) >= maxUniqueURLs
			visitMu.Unlock()
			if over {
				break
			}
			wg.Add(1)
			go crawl(link, depth+1)
		}
	}

	wg.Add(1)
	go crawl(startURL, 0)
	wg.Wait()

	log.Debug("crawl finished", zap.Int("urls", len(urls)), zap.Int("errors", len(errs)))
	return Result{URLs: urls, Errors: errs, Seed: seed, Meta: meta}, nil
}

func fetchAndExtractWithBrowser(ctx context.Context, browser web.BrowserFetcher, fetchURL, original, rootHost string, timeout time.Duration) ([]string, *repo.FetchDetails, *repo.HTMLMetadata, error) {
	details := &repo.FetchDetails{RequestedURL: fetchURL}
	start := time.Now().UTC()

	bctx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		bctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var finalURL, htmlStr string
	var err error
	if bs, ok := browser.(web.BrowserFetcherWithStrategy); ok {
		finalURL, htmlStr, err = bs.FetchRenderedHTMLWithStrategy(bctx, fetchURL, web.NavStrategyCDP)
		if err != nil && strings.Contains(err.Error(), "ERR_HTTP2_PROTOCOL_ERROR") {
			// Retry once using a different navigation strategy.
			finalURL, htmlStr, err = bs.FetchRenderedHTMLWithStrategy(bctx, fetchURL, web.NavStrategyNavigate)
		}
	} else {
		finalURL, htmlStr, err = browser.FetchRenderedHTML(bctx, fetchURL)
		if err != nil && strings.Contains(err.Error(), "ERR_HTTP2_PROTOCOL_ERROR") {
			finalURL, htmlStr, err = browser.FetchRenderedHTML(bctx, fetchURL)
		}
	}
	details.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		details.Error = err.Error()
		return nil, details, nil, fmt.Errorf("browser fetch %s: %w", fetchURL, err)
	}
	details.FinalURL = finalURL

	// Parse rendered HTML for links + metadata.
	pageURL := original
	if finalURL != "" {
		pageURL = finalURL
	}
	effectiveRootHost := rootHost
	if pageURL != "" {
		if h, herr := web.Hostname(pageURL); herr == nil && h != "" {
			effectiveRootHost = h
		}
	}
	links, meta, exErr := extractLinksAndMetadata(strings.NewReader(htmlStr), pageURL, effectiveRootHost, 2000)
	if exErr != nil {
		details.Error = exErr.Error()
		return links, details, meta, exErr
	}
	return links, details, meta, nil
}

func fetchAndExtract(ctx context.Context, client *http.Client, fetchURL, original, rootHost string, log *zap.Logger) ([]string, *repo.FetchDetails, *repo.HTMLMetadata, error) {
	details := &repo.FetchDetails{RequestedURL: fetchURL}
	start := time.Now().UTC()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		details.Error = err.Error()
		details.DurationMs = time.Since(start).Milliseconds()
		return nil, details, nil, fmt.Errorf("request %s: %w", fetchURL, err)
	}
	req.Header.Set("User-Agent", "web_crawler/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	if log != nil {
		log.Debug("http request",
			zap.String("url", fetchURL),
			zap.String("method", req.Method),
			zap.String("host", req.URL.Host),
		)
	}

	res, err := client.Do(req)
	if err != nil {
		details.Error = err.Error()
		details.DurationMs = time.Since(start).Milliseconds()
		return nil, details, nil, fmt.Errorf("fetch %s: %w", fetchURL, err)
	}
	defer res.Body.Close()

	if res.Request != nil && res.Request.URL != nil {
		details.FinalURL = res.Request.URL.String()
	}
	details.StatusCode = res.StatusCode
	details.Status = res.Status
	details.ContentType = res.Header.Get("Content-Type")

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Keep body snippets small to avoid bloating API responses.
		snippet, _ := readBodySnippet(res.Body, 512)
		details.BodySnippet = snippet
		details.DurationMs = time.Since(start).Milliseconds()
		if log != nil {
			log.Debug("non-2xx response",
				zap.String("url", fetchURL),
				zap.Int("status_code", res.StatusCode),
				zap.String("status", res.Status),
				zap.String("content_type", res.Header.Get("Content-Type")),
				zap.String("server", res.Header.Get("Server")),
				zap.String("via", res.Header.Get("Via")),
				zap.String("x_amz_rid", res.Header.Get("x-amz-rid")),
				zap.String("body_snippet", snippet),
			)
		}
		details.Error = fmt.Sprintf("status %s", res.Status)
		return nil, details, nil, fmt.Errorf("fetch %s: status %s", fetchURL, res.Status)
	}

	ct := res.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(strings.ToLower(ct), "text/html") {
		_, _ = io.Copy(io.Discard, res.Body)
		details.Skipped = true
		details.DurationMs = time.Since(start).Milliseconds()
		if log != nil {
			log.Debug("skipping non-html", zap.String("url", fetchURL), zap.String("content_type", ct))
		}
		return nil, details, nil, nil
	}

	pageURL := original
	if details.FinalURL != "" {
		pageURL = details.FinalURL
	}
	effectiveRootHost := rootHost
	if pageURL != "" {
		if h, herr := web.Hostname(pageURL); herr == nil && h != "" {
			effectiveRootHost = h
		}
	}
	links, meta, exErr := extractLinksAndMetadata(res.Body, pageURL, effectiveRootHost, 2000)
	details.DurationMs = time.Since(start).Milliseconds()
	if exErr != nil {
		details.Error = exErr.Error()
		return links, details, meta, exErr
	}
	return links, details, meta, nil
}

func readBodySnippet(r io.Reader, limit int64) (string, error) {
	if limit <= 0 {
		return "", nil
	}
	b, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > 1000 {
		s = s[:1000] + "...(truncated)"
	}
	return s, nil
}

func extractLinks(r io.Reader, pageURL, rootHost string) ([]string, error) {
	links, _, err := extractLinksAndMetadata(r, pageURL, rootHost, 0)
	return links, err
}

func extractLinksAndMetadata(r io.Reader, pageURL, rootHost string, bodySnippetLimit int) ([]string, *repo.HTMLMetadata, error) {
	tok := html.NewTokenizer(r)
	var out []string
	meta := &repo.HTMLMetadata{}
	var inTitle bool
	var inBody bool
	var bodySB strings.Builder

	for {
		switch tok.Next() {
		case html.ErrorToken:
			if tok.Err() == io.EOF {
				if meta.Title == "" && meta.Description == "" && meta.BodySnippet == "" {
					return out, nil, nil
				}
				meta.BodySnippet = strings.TrimSpace(meta.BodySnippet)
				return out, meta, nil
			}
			return out, metaOrNil(meta), tok.Err()
		case html.TextToken:
			txt := strings.TrimSpace(string(tok.Text()))
			if txt == "" {
				continue
			}
			if inTitle && meta.Title == "" {
				meta.Title = collapseWhitespace(txt)
				continue
			}
			if inBody && bodySnippetLimit > 0 && bodySB.Len() < bodySnippetLimit {
				needSpace := bodySB.Len() > 0 && !unicode.IsSpace(rune(bodySB.String()[bodySB.Len()-1]))
				if needSpace {
					bodySB.WriteByte(' ')
				}
				bodySB.WriteString(collapseWhitespace(txt))
				if bodySB.Len() > bodySnippetLimit {
					s := bodySB.String()
					bodySB.Reset()
					bodySB.WriteString(s[:bodySnippetLimit])
				}
				meta.BodySnippet = bodySB.String()
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			t := tok.Token()
			switch t.Data {
			case "title":
				inTitle = true
			case "body":
				inBody = true
			case "meta":
				var name, content string
				for _, a := range t.Attr {
					switch strings.ToLower(a.Key) {
					case "name":
						name = strings.ToLower(strings.TrimSpace(a.Val))
					case "content":
						content = strings.TrimSpace(a.Val)
					}
				}
				if name == "description" && meta.Description == "" {
					meta.Description = collapseWhitespace(content)
				}
			case "a":
				for _, a := range t.Attr {
					if a.Key != "href" {
						continue
					}
					if !web.SameRegistrableHost(a.Val, pageURL) {
						continue
					}
					abs, err := web.AbsoluteURL(a.Val, pageURL)
					if err != nil {
						continue
					}
					if !web.IsHTTPScheme(abs) {
						continue
					}
					h, err := web.Hostname(abs)
					if err != nil || h != rootHost {
						continue
					}
					norm, err := web.Normalize(abs)
					if err != nil {
						continue
					}
					out = append(out, norm)
				}
			}
		case html.EndTagToken:
			t := tok.Token()
			switch t.Data {
			case "title":
				inTitle = false
			case "body":
				inBody = false
			}
		}
	}
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func metaOrNil(m *repo.HTMLMetadata) *repo.HTMLMetadata {
	if m == nil {
		return nil
	}
	if m.Title == "" && m.Description == "" && strings.TrimSpace(m.BodySnippet) == "" {
		return nil
	}
	m.BodySnippet = strings.TrimSpace(m.BodySnippet)
	return m
}
