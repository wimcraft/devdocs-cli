package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/wimcraft/devdocs-cli/internal/devdocs"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	code := devdocs.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	os.Exit(code)
}
