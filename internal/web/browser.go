package web

import (
	"context"
	"os"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type NavStrategy string

const (
	NavStrategyCDP      NavStrategy = "cdp_navigate"
	NavStrategyNavigate NavStrategy = "chromedp_navigate"
)

// BrowserFetcher fetches rendered HTML for a URL (JS/cookies capable).
// This is used only for the seed URL when enabled.
type BrowserFetcher interface {
	FetchRenderedHTML(ctx context.Context, url string) (finalURL string, html string, err error)
}

// BrowserFetcherWithStrategy optionally supports alternate navigation strategies.
// Used for targeted retries on browser network errors.
type BrowserFetcherWithStrategy interface {
	FetchRenderedHTMLWithStrategy(ctx context.Context, url string, strategy NavStrategy) (finalURL string, html string, err error)
}

type chromedpFetcher struct{}

func NewChromedpFetcher() BrowserFetcher {
	return &chromedpFetcher{}
}

func (f *chromedpFetcher) FetchRenderedHTML(ctx context.Context, url string) (string, string, error) {
	return f.FetchRenderedHTMLWithStrategy(ctx, url, NavStrategyCDP)
}

func (f *chromedpFetcher) FetchRenderedHTMLWithStrategy(ctx context.Context, url string, strategy NavStrategy) (string, string, error) {
	allocOpts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocOpts = append(allocOpts,
		chromedp.Flag("headless", "new"), // 'new' is better, but 'false' (headed) is safest for debugging
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"),
		chromedp.Flag("disable-blink-features", "AutomationControlled"), // Hides the "webdriver" flag
		chromedp.NoSandbox,
	)
	if p := os.Getenv("CHROME_PATH"); p != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(p))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer allocCancel()

	// Use a separate tab context per request.
	cctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Ensure we don't hang if the parent ctx has no deadline.
	if _, ok := ctx.Deadline(); !ok {
		var ccancel context.CancelFunc
		cctx, ccancel = context.WithTimeout(cctx, 45*time.Second)
		defer ccancel()
	}

	var htmlStr string
	var final string
	var nav chromedp.Action
	switch strategy {
	case NavStrategyNavigate:
		nav = chromedp.Navigate(url)
	default:
		// NOTE: chromedp.Navigate waits for the "load" event, which can hang on some sites.
		// We instead trigger navigation via CDP and only wait for the DOM to exist.
		nav = chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, _, _, err := page.Navigate(url).Do(ctx)
			return err
		})
	}

	err := chromedp.Run(cctx,
		nav,
		// Give redirects / initial DOM a moment.
		chromedp.Sleep(2*time.Second),
		chromedp.WaitReady("html", chromedp.ByQuery),
		chromedp.Location(&final),
		chromedp.OuterHTML("html", &htmlStr, chromedp.ByQuery),
	)
	if err != nil {
		return "", "", err
	}
	return final, htmlStr, nil
}
