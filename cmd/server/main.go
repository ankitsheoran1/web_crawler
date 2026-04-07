package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"ankit/web_crawler/internal/api"
	"ankit/web_crawler/internal/config"
	"ankit/web_crawler/internal/repo"
	"ankit/web_crawler/internal/service"
	"ankit/web_crawler/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	zcfg := zap.NewProductionConfig()
	if lvl := strings.TrimSpace(cfg.LogLevel); lvl != "" {
		if err := zcfg.Level.UnmarshalText([]byte(lvl)); err != nil {
			log.Fatalf("logger level %q: %v", cfg.LogLevel, err)
		}
	} else {
		zcfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}
	zl, err := zcfg.Build()
	if err != nil {
		log.Fatalf("logger: %v", err)
	}
	defer zl.Sync()

	store := repo.NewStore()
	crawlSvc := service.NewCrawl(zl, store, cfg, web.NewHTTPClient(cfg.HTTPTimeout), web.NewChromedpFetcher())

	mux := http.NewServeMux()
	api.NewHandler(zl, cfg, crawlSvc).Register(mux)

	srv := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		zl.Info("listening", zap.String("addr", cfg.ServerAddr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zl.Fatal("server", zap.Error(err))
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
