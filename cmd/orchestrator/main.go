package main

import (
	"context"
	"log"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sandboxd-o/orchestrator/config"
	httpserver "sandboxd-o/orchestrator/http"
	"sandboxd-o/orchestrator/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("orchestrator config error: %v", err)
	}

	svc, err := service.New(cfg)
	if err != nil {
		log.Fatalf("orchestrator init error: %v", err)
	}
	defer svc.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := svc.BootstrapNodes(ctx); err != nil {
		log.Fatalf("bootstrap nodes error: %v", err)
	}
	svc.StartHeartbeatLoop(ctx)

	router := httpserver.NewRouter(svc)
	srv := &nethttp.Server{
		Addr:              svc.HTTPAddr(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != nethttp.ErrServerClosed {
			log.Fatalf("orchestrator server error: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), svc.ShutdownTimeout())
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
