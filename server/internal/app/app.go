package app

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync/atomic"

	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/instance"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/sharding"

	pkgerr "github.com/pkg/errors"
)

const registerGroupName = "flow/instances"

type App struct {
	rootCtx    context.Context
	rootCancel context.CancelFunc

	registry *instance.Registry
	sweeper  *instance.Sweeper
	watcher  *instance.Watcher

	self      *instance.Instance
	selfShard atomic.Pointer[sharding.Shard]
	shardCalc sharding.Calculator
	ready     atomic.Bool

	apiServer *ApiServer
}

func New(repo *repository.Impl) (*App, error) {
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
		registry:  registry,
		sweeper:   sweeper,
		watcher:   watcher,
		shardCalc: &sharding.SequentialCalculator{},
	}
	a.rootCtx, a.rootCancel = context.WithCancel(context.Background())

	apiServer, err := NewApiServer(
		a.rootCtx,
		config.Conf.ApiServer,
		repo.Store(),
	)
	if err != nil {
		return nil, err
	}
	a.apiServer = apiServer

	return a, nil
}

func (a *App) bootstrap() error {
	a.sweeper.Start(a.rootCtx)
	a.startInstanceWatch()

	newInst, err := a.registry.Register(a.rootCtx, registerGroupName)
	if err != nil {
		return pkgerr.WithMessage(err, "register instance failed")
	}

	a.self = newInst
	if err := a.updateShard(a.rootCtx); err != nil {
		return pkgerr.WithMessage(err, "init shard failed")
	}

	a.ready.Store(true)

	return nil
}

func (a *App) startInstanceWatch() {
	watchCh := a.watcher.Watch(a.rootCtx, registerGroupName)
	go func() {
		defer func() {
			if e := recover(); e != nil {
				slog.ErrorContext(a.rootCtx,
					"instance watcher panic",
					slog.Any("err", e),
					slog.String("stacks", string(debug.Stack())),
				)
			}
		}()

		for resp := range watchCh {
			if err := resp.Err(); err != nil {
				slog.ErrorContext(a.rootCtx,
					"instance watcher exited",
					slog.Any("err", err),
				)
				return
			}

			for _, event := range resp.Events {
				if err := a.onInstanceEvent(a.rootCtx, event); err != nil {
					slog.ErrorContext(a.rootCtx,
						"handle instance event failed",
						slog.Any("err", err),
					)
				}
			}
		}
	}()
}

func (a *App) Run() {
	errCh := make(chan error)
	go func() {
		errCh <- a.run()
	}()

	if err := waitExitSignal(errCh); err != nil {
		slog.Error("app exit due to error", slog.Any("err", err))
	}

	a.close()
}

func (a *App) startApiServer() error {
	return a.apiServer.Spin()
}

func (a *App) run() error {
	err := a.bootstrap()
	if err != nil {
		return pkgerr.WithMessage(err, "bootstrap failed")
	}

	return a.startApiServer()
}

func (a *App) close() {
	a.ready.Store(false)
	a.rootCancel()
	a.apiServer.Stop()
	a.registry.Close()
	a.sweeper.Close()

	slog.Info("app closed")
}

func (a *App) updateShard(ctx context.Context) error {
	// 取全量
	peers, err := a.registry.GetAllPeers(ctx)
	if err != nil {
		// 取全量失败维持不变
		slog.ErrorContext(ctx, "get all peers failed", slog.Any("err", err))
		return pkgerr.WithMessage(err, "get all peers failed")
	}

	selfId := a.self.GetId()
	peerIds := make([]int64, 0, len(peers))
	for _, peer := range peers {
		peerIds = append(peerIds, peer.GetId())
	}

	newShard := a.shardCalc.GetShard(selfId, peerIds)
	if !newShard.Valid() {
		slog.WarnContext(ctx, "new shard is not valid, skip instance event")
		return pkgerr.New("new shard is not valid")
	}
	if a.self.GetId() != newShard.Id {
		slog.ErrorContext(ctx,
			"new shard is not valid, skip instance event",
			slog.Any("newShard", newShard),
		)
		return pkgerr.New("new shard is not valid")
	}

	a.selfShard.Store(newShard)

	return nil
}

func (a *App) onInstanceEvent(ctx context.Context, event *instance.InstanceEvent) error {
	slog.DebugContext(ctx, "instance event", slog.Any("event", event))
	if !a.ready.Load() {
		slog.WarnContext(ctx, "app register is not ready, skip instance event")
		return nil
	}

	if err := a.updateShard(ctx); err != nil {
		slog.ErrorContext(ctx, "update shard failed", slog.Any("err", err))
		return pkgerr.WithMessage(err, "update shard failed")
	}

	return nil
}
