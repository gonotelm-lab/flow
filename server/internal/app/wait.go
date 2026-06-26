package app

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func waitSignal(errCh chan error) error {
	signalToNotify := []os.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM}
	if signal.Ignored(syscall.SIGHUP) {
		signalToNotify = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, signalToNotify...)

	select {
	case sig := <-signals:
		switch sig {
		case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM:
			slog.Info(fmt.Sprintf("Received signal: %s\n", sig))
			// graceful shutdown
			return nil
		}
	case err := <-errCh:
		// error occurs, exit immediately
		return err
	}

	return nil
}
