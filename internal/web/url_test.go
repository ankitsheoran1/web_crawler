package web

import (
	"testing"
	"time"
)

func TestHostname(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"https with host", "https://example.com/path", "example.com", false},
		{"http with port", "http://localhost:8080/", "localhost", false},
		{"missing host", "https:///path", "", true},
		{"relative", "/foo", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Hostname(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Hostname() err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("Hostname() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsHTTPScheme(t *testing.T) {
	t.Parallel()
	if !IsHTTPScheme("https://a.b") {
		t.Fatal("expected https true")
	}
	if IsHTTPScheme("mailto:x@y") {
		t.Fatal("expected mailto false")
	}
}

func TestAbsoluteURL(t *testing.T) {
	t.Parallel()
	base := "https://example.com/dir/page?q=1"
	got, err := AbsoluteURL("/other", base)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.com/other"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got, err = AbsoluteURL("https://other.com/", base)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://other.com/" {
		t.Fatalf("got %q", got)
	}
}

func TestSameRegistrableHost(t *testing.T) {
	t.Parallel()
	base := "https://example.com/x"
	if !SameRegistrableHost("/rel", base) {
		t.Fatal("relative should match for resolution on same page")
	}
	if !SameRegistrableHost("https://example.com/y", base) {
		t.Fatal("same host should match")
	}
	if SameRegistrableHost("https://evil.com/y", base) {
		t.Fatal("other host should not match")
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()
	got, err := Normalize("https://example.com/a#frag")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.com/a"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNewHTTPClient(t *testing.T) {
	t.Parallel()
	c := NewHTTPClient(5 * time.Second)
	if c.Timeout != 5*time.Second {
		t.Fatalf("timeout = %v", c.Timeout)
	}
}
