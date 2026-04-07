package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

const envPrefix = "CRAWLER_"

// DefaultConfigPath is used when CRAWLER_CONFIG is unset.
const DefaultConfigPath = "config.yaml"

// Config holds application defaults and HTTP server settings.
type Config struct {
	MaxDepth       int           `yaml:"-" json:"max_depth"`
	Workers        int           `yaml:"-" json:"workers"`
	URLCooldown    time.Duration `yaml:"-" json:"url_cooldown"`
	HTTPTimeout    time.Duration `yaml:"-" json:"-"`
	ServerAddr     string        `yaml:"-" json:"-"`
	LogLevel       string        `yaml:"-" json:"-"`
	RequestTimeout time.Duration `yaml:"-" json:"-"`
	SeedBrowser        bool          `yaml:"-" json:"-"`
	SeedBrowserTimeout time.Duration `yaml:"-" json:"-"`
}

// fileRoot is the YAML document shape.
type fileRoot struct {
	Crawler crawlerYAML `yaml:"crawler"`
	Server  serverYAML  `yaml:"server"`
}

type crawlerYAML struct {
	MaxDepth       *int   `yaml:"max_depth"`
	Workers        *int   `yaml:"workers"`
	URLCooldown    string `yaml:"url_cooldown"`
	HTTPTimeout    string `yaml:"http_timeout"`
	RequestTimeout string `yaml:"request_timeout"`
}

type serverYAML struct {
	Addr *string `yaml:"addr"`
	LogLevel *string `yaml:"log_level"`
}

// Load reads config.yaml (or CRAWLER_CONFIG), then applies CRAWLER_* env overrides.
func Load() (Config, error) {
	path := os.Getenv(envPrefix + "CONFIG")
	if path == "" {
		path = DefaultConfigPath
	}
	return LoadPath(path)
}

// LoadPath reads YAML from path and applies environment overrides.
func LoadPath(path string) (Config, error) {
	c, err := loadYAML(path)
	if err != nil {
		return Config{}, err
	}
	applyEnv(&c)
	if err := validate(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func loadYAML(path string) (Config, error) {
	abs := path
	if !filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err != nil {
			return Config{}, fmt.Errorf("getwd: %w", err)
		}
		abs = filepath.Join(wd, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", abs, err)
	}

	var root fileRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return Config{}, fmt.Errorf("parse yaml %s: %w", abs, err)
	}

	out := Config{}
	if root.Crawler.MaxDepth != nil {
		out.MaxDepth = *root.Crawler.MaxDepth
	} else {
		out.MaxDepth = 3
	}
	if root.Crawler.Workers != nil {
		out.Workers = *root.Crawler.Workers
	} else {
		out.Workers = 10
	}
	if root.Server.Addr != nil {
		out.ServerAddr = *root.Server.Addr
	}
	if root.Server.LogLevel != nil {
		out.LogLevel = *root.Server.LogLevel
	}
	if root.Crawler.URLCooldown != "" {
		d, err := time.ParseDuration(root.Crawler.URLCooldown)
		if err != nil {
			return Config{}, fmt.Errorf("crawler.url_cooldown: %w", err)
		}
		out.URLCooldown = d
	} else {
		out.URLCooldown = 500 * time.Millisecond
	}
	if root.Crawler.HTTPTimeout != "" {
		d, err := time.ParseDuration(root.Crawler.HTTPTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("crawler.http_timeout: %w", err)
		}
		out.HTTPTimeout = d
	} else {
		out.HTTPTimeout = 60 * time.Second
	}
	if root.Crawler.RequestTimeout != "" {
		d, err := time.ParseDuration(root.Crawler.RequestTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("crawler.request_timeout: %w", err)
		}
		out.RequestTimeout = d
	} else {
		out.RequestTimeout = 300 * time.Second
	}
	if out.ServerAddr == "" {
		out.ServerAddr = ":8080"
	}
	if out.LogLevel == "" {
		out.LogLevel = "info"
	}
	return out, nil
}

func applyEnv(c *Config) {
	if v := os.Getenv(envPrefix + "MAX_DEPTH"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n >= 0 {
			c.MaxDepth = n
		}
	}
	if v := os.Getenv(envPrefix + "WORKERS"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n >= 1 {
			c.Workers = n
		}
	}
	if v := os.Getenv(envPrefix + "URL_COOLDOWN"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			c.URLCooldown = d
		}
	}
	if v := os.Getenv(envPrefix + "HTTP_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.HTTPTimeout = d
		}
	}
	if v := os.Getenv(envPrefix + "SERVER_ADDR"); v != "" {
		c.ServerAddr = v
	}
	if v := os.Getenv(envPrefix + "LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}
	if v := os.Getenv(envPrefix + "REQUEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.RequestTimeout = d
		}
	}
}

func validate(c Config) error {
	if c.MaxDepth < 0 {
		return fmt.Errorf("max_depth must be >= 0")
	}
	if c.Workers < 1 {
		return fmt.Errorf("workers must be >= 1")
	}
	if c.URLCooldown < 0 {
		return fmt.Errorf("url_cooldown must be non-negative")
	}
	if c.ServerAddr == "" {
		return fmt.Errorf("server.addr is required")
	}
	if c.LogLevel == "" {
		return fmt.Errorf("server.log_level is required")
	}
	return nil
}
