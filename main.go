package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"registry-dns-switcher/internal/app"
	"registry-dns-switcher/internal/config"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	once := flag.Bool("once", false, "run one reconciliation and exit")
	dryRun := flag.Bool("dry-run", false, "select target IP without changing DNS")

	flag.Parse()

	var opts []config.LoadOption
	if *once {
		opts = append(opts, config.WithOnce(true))
	}

	if *dryRun {
		opts = append(opts, config.WithDryRun(true))
	}

	cfg, err := config.LoadFile(*configPath, opts...)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switcher, err := app.New(cfg)
	if err != nil {
		slog.Error("failed to create switcher", "error", err)
		os.Exit(1)
	}

	if cfg.Run.Once {
		if err := switcher.Reconcile(ctx); err != nil {
			slog.Error("reconcile failed", "error", err)
			os.Exit(1)
		}

		return
	}

	interval := cfg.Run.Interval
	if interval <= 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := switcher.Reconcile(ctx); err != nil {
			slog.Error("reconcile failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
