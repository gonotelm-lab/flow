package app

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/audit"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/endpoint"
	"github.com/gonotelm-lab/flow/server/internal/instance"
	"github.com/gonotelm-lab/flow/server/internal/otel"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/scheduler"
	"github.com/gonotelm-lab/flow/server/internal/sharding"
	"github.com/gonotelm-lab/flow/server/internal/taskmender"

	"github.com/pkg/errors"
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

	apiServer *endpoint.ApiServer

	retryMender   *taskmender.RetryMender
	staleDetector *taskmender.StaleDetector

	// dispatcher 为 nil 时不启动定时回调调度（config.Conf.Scheduler 为空）。
	dispatcher *scheduler.Dispatcher
	// auditRec 是所有 admin 操作审计的异步写入器，进程退出前必须 flush。
	auditRec *audit.Recorder

	store *repository.Store
}

func New(repo *repository.Impl, rootCtx context.Context) (*App, error) {
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
		store:     repo.Store(),
	}
	a.rootCtx, a.rootCancel = context.WithCancel(rootCtx)

	// 审计写入是异步的，这里只构造；实际写库由 bootstrap 里的 Run 循环驱动。
	auditRec := audit.New(repo.Store().AdminAuditLog, audit.Config{})

	apiServer, err := endpoint.NewApiServer(
		a.rootCtx,
		config.Conf.ApiServer,
		repo.Store(),
		auditRec,
	)
	if err != nil {
		return nil, err
	}
	a.apiServer = apiServer
	a.auditRec = auditRec

	return a, nil
}

func (a *App) bootstrap() error {
	a.sweeper.Start(a.rootCtx)
	a.startInstanceWatch()

	if config.Conf.Worker != nil {
		a.retryMender = taskmender.NewRetryMender(a.store, config.Conf.Worker)
		go a.retryMender.Run(a.rootCtx)

		a.staleDetector = taskmender.NewStaleDetector(a.store, config.Conf.Worker)
		go a.staleDetector.Run(a.rootCtx)
	}

	regValue := fmt.Sprintf("%s:%d", config.Conf.ApiServer.Grpc.Listen, config.Conf.ApiServer.Grpc.Port)
	newInst, err := a.registry.Register(a.rootCtx, registerGroupName, regValue)
	if err != nil {
		return errors.WithMessage(err, "register instance failed")
	}

	a.self = newInst
	if err := a.updateShard(a.rootCtx); err != nil {
		return errors.WithMessage(err, "init shard failed")
	}

	a.ready.Store(true)

	// 审计写入循环：与业务生命周期同进退，close 时会 flush 剩余条目。
	go a.auditRec.Run(a.rootCtx)

	// 调度器必须在 updateShard 之后启动：它读的就是这里刚算出来的分片区间。
	// 之后每次实例变更，watcher 都会刷新 a.selfShard，dispatcher 每 tick 现读，
	// 因此扩缩容不需要任何额外的重平衡通知。
	if config.Conf.Scheduler != nil {
		a.dispatcher = scheduler.NewDispatcher(
			a.store,
			config.Conf.Scheduler,
			func() *sharding.Shard { return a.selfShard.Load() },
			fmt.Sprintf("instance-%d", a.self.GetId()),
		)
		go a.dispatcher.Run(a.rootCtx)
	}

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
				slog.ErrorContext(a.rootCtx, "instance watcher exited", slog.Any("err", err))
				return
			}

			for _, event := range resp.Events {
				if err := a.onInstanceEvent(a.rootCtx, event); err != nil {
					slog.ErrorContext(a.rootCtx, "handle instance event failed", slog.Any("err", err))
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

	if err := waitSignal(errCh); err != nil {
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
		return errors.WithMessage(err, "bootstrap failed")
	}

	return a.startApiServer()
}

func (a *App) close() {
	a.ready.Store(false)
	a.apiServer.Stop()
	a.registry.Close()
	a.sweeper.Close()
	if a.retryMender != nil {
		a.retryMender.Close()
	}
	if a.staleDetector != nil {
		a.staleDetector.Close()
	}
	if a.dispatcher != nil {
		a.dispatcher.Close()
	}
	a.rootCancel()

	// 放在 rootCancel 之后：Run 会在 ctx 结束时退出循环，Close 再把队列里剩下的
	// 审计条目 flush 掉，避免关机丢掉「谁在什么时候删了什么」。
	if a.auditRec != nil {
		a.auditRec.Close()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	otel.Shutdown(shutdownCtx)

	slog.Info("app closed")
}

func (a *App) updateShard(ctx context.Context) error {
	// 取全量
	peers, err := a.registry.GetAllPeers(ctx)
	if err != nil {
		// 取全量失败维持不变
		slog.ErrorContext(ctx, "get all peers failed", slog.Any("err", err))
		return errors.WithMessage(err, "get all peers failed")
	}

	selfId := a.self.GetId()
	peerIds := make([]int64, 0, len(peers))
	for _, peer := range peers {
		peerIds = append(peerIds, peer.GetId())
	}

	newShard := a.shardCalc.GetShard(selfId, peerIds)
	if !newShard.Valid() {
		slog.WarnContext(ctx, "new shard is not valid, skip instance event")
		return errors.New("new shard is not valid")
	}
	if a.self.GetId() != newShard.Id {
		slog.ErrorContext(ctx,
			"new shard is not valid, skip instance event",
			slog.Any("newShard", newShard),
		)
		return errors.New("new shard is not valid")
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
		return errors.WithMessage(err, "update shard failed")
	}

	return nil
}
