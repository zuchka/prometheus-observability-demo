package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zuchka/prometheus-observability-demo/internal/demoapp"
)

func main() {
	cfg := demoapp.DefaultConfig()
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	flag.StringVar(&cfg.AdminToken, "admin-token", cfg.AdminToken, "admin token for /chaos")
	flag.StringVar(&cfg.TempDir, "temp-dir", cfg.TempDir, "directory for bounded temporary I/O work")
	maxMemoryMB := flag.Int64("max-memory-mb", cfg.MaxMemoryBytes/1024/1024, "maximum memory allocation per /api/memory request")
	flag.Parse()
	cfg.MaxMemoryBytes = *maxMemoryMB * 1024 * 1024

	app, err := demoapp.New(cfg)
	if err != nil {
		log.Fatalf("create app: %v", err)
	}

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("demo-api listening on http://%s", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
