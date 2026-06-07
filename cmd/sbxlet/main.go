package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sandboxd-o/pkg/logging"
	"sandboxd-o/sandboxd-let/config"
	_ "sandboxd-o/sandboxd-let/docs"
	httpserver "sandboxd-o/sandboxd-let/http"
	"sandboxd-o/sandboxd-let/sandbox"
)

// @title Sandboxd(Node Agent) API Server
// @version 1.0
// @BasePath /
// @schemes http

func main() {
	configPath := config.DefaultConfigPath
	configPathShort := ""
	flag.StringVar(&configPath, "config", config.DefaultConfigPath, "path to sbxlet config json")
	flag.StringVar(&configPathShort, "c", "", "shorthand for --config")
	flag.Parse()
	if strings.TrimSpace(configPathShort) != "" {
		configPath = configPathShort
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		boot := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		boot.Error("sbxlet config error", slog.Any("error", err))
		os.Exit(1)
	}

	logger, err := logging.New(logging.Config{
		Dir:        cfg.LogDir,
		FilePrefix: valueOrDefault(cfg.LogFilePrefix, "sandboxd"),
	}, logging.Options{Service: "sandboxd", Env: strings.TrimSpace(cfg.AppEnv), AddSource: false, Level: slog.LevelInfo})
	if err != nil {
		boot := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		boot.Error("sandboxd logging init error", slog.Any("error", err))
		os.Exit(1)
	}
	defer logger.Close()

	slog.SetDefault(logger.Logger)
	log.SetOutput(logger)
	log.SetFlags(0)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	svc, err := sandbox.New(ctx, cfg)
	if err != nil {
		logger.Error("init sandbox service error", slog.Any("error", err))
		os.Exit(1)
	}

	defer svc.Close()
	svc.StartReconcileLoop(ctx)

	h := httpserver.New(svc, logger).Handler()
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("http server failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func valueOrDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}

	return v
}
