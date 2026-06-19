package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/instance"
	"github.com/gonotelm-lab/flow/server/internal/repository"
)

const registerGroupName = "flow/instances"

type App struct {
	rootCtx    context.Context
	rootCancel context.CancelFunc

	registry *instance.Registry
	sweeper  *instance.Sweeper
	watcher  *instance.Watcher
}

func New() *App {
	repo := repository.Repository()
	registry := instance.NewRegistry(
		repo.TxManager(),
		repo.Store(),
		instance.RegistryConfig{
			Expiry:            config.Conf.Registry.Expiry,
			KeepaliveInterval: config.Conf.Registry.KeepaliveInterval,
		},
	)

	sweeper := instance.NewSweeper(
		repo.TxManager(),
		repo.Store(),
		instance.SweeperConfig{
			Interval:  config.Conf.Registry.SweepInterval,
			BatchSize: config.Conf.Registry.SweepBatch,
		},
	)

	watcher := instance.NewWatcher(
		repo.Store(),
		instance.WatcherConfig{
			Interval:        config.Conf.Registry.WatchInterval,
			BatchSize:       config.Conf.Registry.WatchBatchSize,
			MaxRetryBackoff: config.Conf.Registry.WatchMaxRetryBackoff,
		},
	)

	a := &App{
		registry: registry,
		sweeper:  sweeper,
		watcher:  watcher,
	}

	a.rootCtx, a.rootCancel = context.WithCancel(context.Background())

	return a
}

func (a *App) bootstrap() {
	_, err := a.registry.Register(a.rootCtx, registerGroupName)
	if err != nil {
		panic(fmt.Errorf("register instance failed: %w", err))
	}

	a.sweeper.Start(a.rootCtx)

	a.watcher.Watch(a.rootCtx, registerGroupName, a.onInstanceEvent)
}

func (a *App) Run() {
	a.bootstrap()

	select {}
}

func (a *App) onInstanceEvent(ctx context.Context, event *instance.InstanceEvent) error {
	slog.InfoContext(ctx, "instance event", "event", event)
	return nil
}
