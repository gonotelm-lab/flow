package instance

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"

	pkgerr "github.com/pkg/errors"
)

const (
	discovRevisionName = "flow/discovery/revision"
)

func zeroRevision() *schema.GlobalRevision {
	return &schema.GlobalRevision{
		Name:            discovRevisionName,
		CurrentRevision: 0,
		UpdateTime:      time.Now().UnixMilli(),
	}
}

type Registry struct {
	txMgr repository.TxManager
	store repository.Store

	closing   atomic.Bool
	mu        sync.RWMutex
	instances map[int64]*cancellableInstance
	wg        sync.WaitGroup
}

func NewRegistry(
	txMgr repository.TxManager,
	store repository.Store,
) *Registry {
	return &Registry{
		txMgr:     txMgr,
		store:     store,
		instances: make(map[int64]*cancellableInstance),
	}
}

// 注册当前服务
func (r *Registry) Register(
	ctx context.Context,
) (Instance, error) {
	if r.closing.Load() {
		return Instance{}, pkgerr.New("registry is closing")
	}

	nowMs := time.Now().UnixMilli()
	zero := zeroRevision()

	var instance *Instance

	err := r.txMgr.Transact(ctx, func(ctx context.Context) error {
		// 1. get global revision
		curRev, err := r.store.GlobalRevision.GetOrInitForUpdate(ctx, zero)
		if err != nil {
			return pkgerr.WithMessage(err, "get global revision failed")
		}

		// 2. insert instance
		revison := curRev.CurrentRevision + 1
		instance = NewInstance(revison)
		created, err := r.store.Instance.Create(ctx, instance.ToSchema())
		if err != nil {
			return pkgerr.WithMessage(err, "create instance failed")
		}

		instance.Id = created.Id

		// 3. insert instance event
		err = r.store.InstanceEvent.Append(ctx, &schema.InstanceEvent{
			Revision:   revison,
			Group:      instance.Group,
			Key:        instance.Key,
			Value:      instance.Value,
			Type:       InstanceEventPut.String(),
			CreateTime: nowMs,
		})
		if err != nil {
			return pkgerr.WithMessage(err, "append instance event failed")
		}

		// 4. update global revision
		err = r.store.GlobalRevision.IncrRevision(ctx, discovRevisionName, nowMs)
		if err != nil {
			return pkgerr.WithMessage(err, "update global revision failed")
		}

		return nil
	})
	if err != nil {
		return Instance{}, pkgerr.WithMessage(err, "transaction failed")
	}

	cancellableInstance, err := r.keepalive(ctx, instance)
	if err != nil {
		return Instance{}, pkgerr.WithMessage(err, "keep alive instance failed")
	}

	r.mu.Lock()
	r.instances[instance.Id] = cancellableInstance
	r.mu.Unlock()

	return *instance, nil
}

func (r *Registry) Unregister(
	ctx context.Context,
	instanceId int64,
) error {
	r.mu.RLock()
	instance, ok := r.instances[instanceId]
	if !ok {
		r.mu.RUnlock()
		return nil
	}
	r.mu.RUnlock()

	err := r.txMgr.Transact(ctx, func(ctx context.Context) error {
		// 0. get global revision
		curRev, err := r.store.GlobalRevision.GetOrInitForUpdate(ctx, zeroRevision())
		if err != nil {
			return pkgerr.WithMessage(err, "get global revision failed")
		}

		revision := curRev.CurrentRevision + 1

		// 1. delete instance
		err = r.store.Instance.Delete(ctx, instance.Id)
		if err != nil {
			return pkgerr.WithMessage(err, "delete instance failed")
		}

		// 2. put event
		err = r.store.InstanceEvent.Append(ctx, &schema.InstanceEvent{
			Revision:   revision,
			Group:      instance.Group,
			Key:        instance.Key,
			Value:      instance.Value,
			Type:       InstanceEventDelete.String(),
			CreateTime: time.Now().UnixMilli(),
		})
		if err != nil {
			return pkgerr.WithMessage(err, "append instance event failed")
		}

		// 3. update global revision
		err = r.store.GlobalRevision.IncrRevision(ctx, discovRevisionName, time.Now().UnixMilli())
		if err != nil {
			return pkgerr.WithMessage(err, "update global revision failed")
		}

		return nil
	})
	if err != nil {
		return pkgerr.WithMessage(err, "transaction failed")
	}

	instance.cancel()
	r.mu.Lock()
	delete(r.instances, instanceId)
	r.mu.Unlock()

	return nil
}

// 关闭registry 所有协程停止心跳推出
func (r *Registry) Close() {
	if !r.closing.CompareAndSwap(false, true) {
		return
	}

	go func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		for _, inst := range r.instances {
			inst.cancel()
		}
	}()

	r.wg.Wait()

	r.mu.Lock()
	r.instances = make(map[int64]*cancellableInstance)
	r.mu.Unlock()
}

func (r *Registry) keepalive(
	ctx context.Context,
	instance *Instance,
) (*cancellableInstance, error) {
	ctx, cancel := context.WithCancel(ctx)
	cInst := &cancellableInstance{
		Instance: instance,
		cancel:   cancel,
	}

	ticker := time.NewTicker(defaultKeepaliveInterval)

	r.wg.Add(1)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				// panic
				slog.ErrorContext(ctx, "keepalive loop panic",
					slog.Any("err", err),
					slog.String("stack", string(debug.Stack())),
				)
			}

			ticker.Stop()
			newCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			r.Unregister(newCtx, instance.Id)
			r.wg.Done()
		}()

		for {
			select {
			case <-ctx.Done():
				slog.InfoContext(ctx, "keepalive loop stopped",
					slog.String("instance", instance.Key),
					slog.Int64("instance_id", instance.Id),
				)
				return
			case <-ticker.C:
				r.heartbeat(ctx, instance)
			}
		}
	}()

	return cInst, nil
}

func (r *Registry) heartbeat(
	ctx context.Context,
	instance *Instance,
) {
	const retryCnt = 3
	oldExpireTime := instance.ExpireTime
	for range retryCnt {
		instance.ExtendTTL(defaultExpiry)
		ok, err := r.store.Instance.UpdateExpireTime(
			ctx, instance.Id,
			instance.ExpireTime,
			instance.FencingToken,
		)
		if err != nil {
			slog.ErrorContext(ctx, "update instance expire time failed", slog.Any("err", err))
			// 失败重置过期时间
			instance.SetExpireTime(oldExpireTime)
			continue
		}

		if !ok {
			// fencing_token mismatch
			slog.ErrorContext(ctx, "fencing token mismatch",
				slog.Int64("instance_id", instance.Id),
				slog.Int64("fencing_token", instance.FencingToken),
			)
		}

		return
	}
}
