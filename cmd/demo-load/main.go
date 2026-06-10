package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/zuchka/prometheus-observability-demo/internal/loadgen"
)

func main() {
	target := envString("DEMO_TARGET_URL", "http://127.0.0.1:8080")
	adminToken := os.Getenv("DEMO_ADMIN_TOKEN")
	profile := envString("DEMO_LOAD_PROFILE", "loop")
	maxConcurrent := envInt("DEMO_LOAD_MAX_CONCURRENT", 12)
	stepDuration := time.Duration(0)
	once := false

	flag.StringVar(&target, "target", target, "demo-api base URL")
	flag.StringVar(&adminToken, "admin-token", adminToken, "admin token for demo-api /chaos")
	flag.StringVar(&profile, "profile", profile, "traffic profile: loop, baseline, burst, error-storm, latency-spike, cpu-io-pulse, recovery")
	flag.IntVar(&maxConcurrent, "max-concurrent", maxConcurrent, "maximum in-flight generated requests")
	flag.DurationVar(&stepDuration, "step-duration", stepDuration, "override every selected step duration, for example 30s")
	flag.BoolVar(&once, "once", once, "run the selected sequence once and exit")
	flag.Parse()

	steps, err := loadgen.Sequence(profile)
	if err != nil {
		log.Fatal(err)
	}
	if stepDuration > 0 {
		for i := range steps {
			steps[i].Duration = stepDuration
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	gen := loadgen.New(loadgen.Config{
		Target:        target,
		AdminToken:    adminToken,
		MaxConcurrent: maxConcurrent,
	})
	loop := !once
	if err := gen.RunSequence(ctx, steps, loop, log.Printf); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
