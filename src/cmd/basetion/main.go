package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/naifenmizuha/basetion/src/internal/bootstrap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(bootstrap.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
