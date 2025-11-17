package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// SignalContext returns a context.Context that is being canceled on SIGINT or
// SIGTERM signals. If there a two signals sent, the whole program is being
// exited with exit code 1.
func SignalContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()

		<-sigChan
		os.Exit(1)
	}()

	return ctx
}
