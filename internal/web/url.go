package web

import (
	"errors"
	"net/http"
	"net/url"
	"time"
)

var ErrInvalidHost = errors.New("hostname is invalid; URL must use http or https with a host")

func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func Hostname(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Hostname() == "" {
		return "", ErrInvalidHost
	}
	return u.Hostname(), nil
}

func IsHTTPScheme(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func AbsoluteURL(ref, base string) (string, error) {
	parsed, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}
	root, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	return root.ResolveReference(parsed).String(), nil
}

// SameRegistrableHost compares hostname of link to rootURL's host (simple equality).
func SameRegistrableHost(link, rootURL string) bool {
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	if !u.IsAbs() {
		return true
	}
	domain, err := Hostname(rootURL)
	if err != nil {
		return false
	}
	return u.Hostname() == domain
}

// Normalize strips fragment and trailing noise for deduplication keys.
func Normalize(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	u.Fragment = ""
	u.RawFragment = ""
	return u.String(), nil
}
