package instance

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"

	"github.com/pkg/errors"
)

const (
	discovRevisionName       = "flow/discovery/revision"
	defaultRegistryExpiry    = time.Second * 12
	defaultKeepaliveInterval = time.Second * 10
)

func zeroRevision() *schema.GlobalRevision {
	return &schema.GlobalRevision{
		Name:            discovRevisionName,
		CurrentRevision: 0,
		UpdateTime:      nowUnixMilli(),
	}
}

// 按create_time(start_time) 从小到大稳定排序返回。
func sortInstances(instances []*Instance) {
	sort.SliceStable(instances, func(i, j int) bool {
		if instances[i].startTime == instances[j].startTime {
			return instances[i].id < instances[j].id
		}
		return instances[i].startTime < instances[j].startTime
	})
}

type Registry struct {
	txMgr *repository.TxManager
	store *repository.Store
	cfg   RegistryConfig

	closing atomic.Bool
	mu      sync.RWMutex
	locals  map[int64]*cancellableInstance
	wg      sync.WaitGroup
}

type RegistryConfig struct {
	Expiry            time.Duration
	KeepaliveInterval time.Duration
}

func (cfg *RegistryConfig) Normalize() {
	if cfg.Expiry <= 0 {
		cfg.Expiry = defaultRegistryExpiry
	}

	if cfg.KeepaliveInterval <= 0 {
		cfg.KeepaliveInterval = defaultKeepaliveInterval
	}
}

func NewRegistry(
	txMgr *repository.TxManager,
	store *repository.Store,
	cfg RegistryConfig,
) *Registry {
	cfg.Normalize()
	return &Registry{
		txMgr:  txMgr,
		store:  store,
		cfg:    cfg,
		locals: make(map[int64]*cancellableInstance),
	}
}

func (r *Registry) registerOnce(
	ctx context.Context,
	group string,
) (*Instance, error) {
	nowMs := nowUnixMilli()
	zero := zeroRevision()

	var instance *Instance

	err := r.txMgr.Transact(ctx, func(ctx context.Context) error {
		// 1. get global revision
		curRev, err := r.store.GlobalRevision.GetOrInitForUpdate(ctx, zero)
		if err != nil {
			return errors.WithMessage(err, "get global revision failed")
		}

		// 2. insert instance
		revision := curRev.CurrentRevision + 1
		instance = NewInstance(group, revision, r.cfg.Expiry)
		created, err := r.store.Instance.Create(ctx, instance.ToSchema())
		if err != nil {
			return errors.WithMessage(err, "create instance failed")
		}

		instance.id = created.Id

		// 3. insert instance event
		err = r.store.InstanceEvent.Append(ctx, &schema.InstanceEvent{
			Revision:   revision,
			Group:      instance.group,
			Key:        instance.key,
			Value:      instance.value,
			Type:       InstanceEventPut.String(),
			CreateTime: nowMs,
		})
		if err != nil {
			return errors.WithMessage(err, "append instance event failed")
		}

		// 4. update global revision
		err = r.store.GlobalRevision.IncrRevision(ctx, discovRevisionName, nowMs)
		if err != nil {
			return errors.WithMessage(err, "update global revision failed")
		}

		return nil
	})
	if err != nil {
		return nil, errors.WithMessage(err, "transaction failed")
	}

	return instance, nil
}

// 注册当前服务
func (r *Registry) Register(
	ctx context.Context,
	group string,
) (*Instance, error) {
	if r.closing.Load() {
		return nil, errors.New("registry is closing")
	}
	group = strings.TrimSpace(group)
	if group == "" {
		return nil, errors.New("registry register group is empty")
	}

	instance, err := r.registerOnce(ctx, group)
	if err != nil {
		return nil, err
	}

	cancellableInstance, err := r.keepalive(ctx, instance)
	if err != nil {
		return nil, errors.WithMessage(err, "keep alive instance failed")
	}

	r.mu.Lock()
	r.locals[instance.id] = cancellableInstance
	r.mu.Unlock()

	return instance, nil
}

// GetAllPeers 返回当前所有远端活跃实例（expire_time > now），
// 并按 create_time(start_time) 从小到大稳定排序返回。
func (r *Registry) GetAllPeers(ctx context.Context) ([]*Instance, error) {
	activeInstances, err := r.store.Instance.ListActive(ctx, nowUnixMilli())
	if err != nil {
		return nil, errors.WithMessage(err, "list active instances failed")
	}

	result := make([]*Instance, 0, len(activeInstances))
	for _, instance := range activeInstances {
		if instance == nil {
			continue
		}
		result = append(result, newInstanceFromSchema(instance))
	}

	sortInstances(result)

	return result, nil
}

func (r *Registry) GetAllLocals(ctx context.Context) ([]*Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Instance, 0, len(r.locals))
	for _, instance := range r.locals {
		result = append(result, instance.Instance)
	}

	sortInstances(result)

	return result, nil
}

func (r *Registry) Unregister(
	ctx context.Context,
	instanceId int64,
) error {
	r.mu.RLock()
	instance, ok := r.locals[instanceId]
	if !ok {
		r.mu.RUnlock()
		return nil
	}
	r.mu.RUnlock()

	err := r.txMgr.Transact(ctx, func(ctx context.Context) error {
		nowMs := nowUnixMilli()

		// 0. get global revision
		curRev, err := r.store.GlobalRevision.GetOrInitForUpdate(ctx, zeroRevision())
		if err != nil {
			return errors.WithMessage(err, "get global revision failed")
		}

		revision := curRev.CurrentRevision + 1

		// 1. delete instance
		err = r.store.Instance.Delete(ctx, instance.id)
		if err != nil {
			return errors.WithMessage(err, "delete instance failed")
		}

		// 2. put event
		err = r.store.InstanceEvent.Append(ctx, &schema.InstanceEvent{
			Revision:   revision,
			Group:      instance.group,
			Key:        instance.key,
			Value:      instance.value,
			Type:       InstanceEventDelete.String(),
			CreateTime: nowMs,
		})
		if err != nil {
			return errors.WithMessage(err, "append instance event failed")
		}

		// 3. update global revision
		err = r.store.GlobalRevision.IncrRevision(ctx, discovRevisionName, nowMs)
		if err != nil {
			return errors.WithMessage(err, "update global revision failed")
		}

		return nil
	})
	if err != nil {
		return errors.WithMessage(err, "transaction failed")
	}

	instance.cancel()
	r.mu.Lock()
	delete(r.locals, instanceId)
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

		for _, inst := range r.locals {
			inst.cancel()
		}
	}()

	r.wg.Wait()

	r.mu.Lock()
	r.locals = make(map[int64]*cancellableInstance)
	r.mu.Unlock()
}

func (r *Registry) keepalive(
	ctx context.Context,
	instance *Instance,
) (*cancellableInstance, error) {
	newCtx, cancel := context.WithCancel(ctx)
	cInst := &cancellableInstance{
		Instance:  instance,
		cancel:    cancel,
		parentCtx: ctx,
	}

	ticker := time.NewTicker(r.cfg.KeepaliveInterval)

	r.wg.Add(1)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				// panic
				slog.ErrorContext(newCtx, "keepalive loop panic",
					slog.Any("err", err),
					slog.String("stack", string(debug.Stack())),
				)
			}

			ticker.Stop()
			// keepalive的ctx此时大概率已经cancelled 所以用一个新的ctx来处理unregister
			unregisterCtx := context.WithoutCancel(ctx)
			unregisterCtx, cc := context.WithTimeout(unregisterCtx, time.Second*5)
			defer cc()
			r.Unregister(unregisterCtx, instance.id)
			r.wg.Done()
		}()

		for {
			select {
			case <-newCtx.Done():
				slog.InfoContext(newCtx, "keepalive loop stopped",
					slog.String("instance", instance.key),
					slog.Int64("instance_id", instance.id),
				)
				return
			case <-ticker.C:
				r.heartbeat(newCtx, instance)
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
	oldExpireTime := instance.expireTime
	for range retryCnt {
		instance.SetExpireTime(nowUnixMilli() + r.cfg.Expiry.Milliseconds())
		ok, err := r.store.Instance.UpdateExpireTime(
			ctx, instance.id,
			instance.expireTime,
			instance.fencingToken,
		)
		if err != nil {
			slog.ErrorContext(ctx, "update instance expire time failed", slog.Any("err", err))
			// 失败重置过期时间
			instance.SetExpireTime(oldExpireTime)
			continue
		}

		// 考虑这样一种场景：
		// 前一时刻sweeper先删掉了instance，后一时刻更新心跳
		// 这里进程并不是挂掉了，而是因为sweeper先删掉了，导致更新失败，尝试自动重新注册
		if !ok {
			slog.WarnContext(ctx, "heartbeat update missed, try auto re-register",
				slog.Int64("instance_id", instance.id),
				slog.Int64("fencing_token", instance.fencingToken),
			)
			if err := r.tryAutoReRegister(ctx, instance); err != nil {
				slog.ErrorContext(ctx, "auto re-register failed", slog.Any("err", err))
			}
		}

		return
	}
}

func (r *Registry) tryAutoReRegister(ctx context.Context, instance *Instance) error {
	if instance == nil {
		return nil
	}

	if r.closing.Load() {
		return nil
	}

	newInst, err := r.registerOnce(ctx, instance.group)
	if err != nil {
		return errors.WithMessage(err, "register replacement instance failed")
	}

	instance.Replace(newInst)

	return nil
}
