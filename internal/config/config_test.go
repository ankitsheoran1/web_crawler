package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadPath_minimalYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	mustWrite(t, path, `
crawler:
  max_depth: 1
  workers: 2
server:
  addr: ":9090"
`)
	c, err := LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxDepth != 1 || c.Workers != 2 {
		t.Fatalf("crawler: %+v", c)
	}
	if c.ServerAddr != ":9090" {
		t.Fatalf("addr %q", c.ServerAddr)
	}
	if c.URLCooldown != 500*time.Millisecond {
		t.Fatalf("default cooldown %v", c.URLCooldown)
	}
}

func TestLoadPath_maxDepthZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	mustWrite(t, path, `
crawler:
  max_depth: 0
  workers: 1
server:
  addr: ":0"
`)
	c, err := LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxDepth != 0 {
		t.Fatalf("max_depth = %d", c.MaxDepth)
	}
}

func TestLoadPath_invalidDuration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	mustWrite(t, path, `
crawler:
  url_cooldown: not-a-duration
server:
  addr: ":8080"
`)
	_, err := LoadPath(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadPath_validationWorkers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	mustWrite(t, path, `
crawler:
  max_depth: 1
  workers: 0
server:
  addr: ":8080"
`)
	_, err := LoadPath(path)
	if err == nil {
		t.Fatal("expected validation error for workers")
	}
}

func TestLoadPath_envOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	mustWrite(t, path, `
crawler:
  max_depth: 1
  workers: 2
server:
  addr: ":8080"
`)
	t.Setenv("CRAWLER_MAX_DEPTH", "7")
	t.Setenv("CRAWLER_WORKERS", "3")
	t.Setenv("CRAWLER_SERVER_ADDR", ":6000")
	t.Setenv("CRAWLER_URL_COOLDOWN", "2s")

	c, err := LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxDepth != 7 || c.Workers != 3 || c.ServerAddr != ":6000" || c.URLCooldown != 2*time.Second {
		t.Fatalf("overrides: %+v", c)
	}
}

func TestLoad_missingFile(t *testing.T) {
	t.Parallel()
	_, err := LoadPath(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want ErrNotExist wrap, got %v", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
