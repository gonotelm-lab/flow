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

// 按 start_time（create_time）从小到大稳定排序。
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
	group, value string,
) (*Instance, error) {
	nowMs := nowUnixMilli()
	zero := zeroRevision()

	var instance *Instance

	err := r.txMgr.Transact(ctx, func(ctx context.Context) error {
		// 1. 取全局 revision
		curRev, err := r.store.GlobalRevision.GetOrInitForUpdate(ctx, zero)
		if err != nil {
			return errors.WithMessage(err, "get global revision failed")
		}

		// 2. 写入 instance
		revision := curRev.CurrentRevision + 1
		value = strings.TrimSpace(value)
		if value == "" {
			value = instanceValue
		}
		instance = NewInstance(group, value, revision, r.cfg.Expiry)
		created, err := r.store.Instance.Create(ctx, instance.ToSchema())
		if err != nil {
			return errors.WithMessage(err, "create instance failed")
		}

		instance.id = created.Id

		// 3. 写 instance event
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

		// 4. 更新全局 revision
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
	group, value string,
) (*Instance, error) {
	if r.closing.Load() {
		return nil, errors.New("registry is closing")
	}
	group = strings.TrimSpace(group)
	if group == "" {
		return nil, errors.New("registry register group is empty")
	}

	instance, err := r.registerOnce(ctx, group, value)
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
// 按 start_time（create_time）从小到大稳定排序。
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

		// 0. 取全局 revision
		curRev, err := r.store.GlobalRevision.GetOrInitForUpdate(ctx, zeroRevision())
		if err != nil {
			return errors.WithMessage(err, "get global revision failed")
		}

		revision := curRev.CurrentRevision + 1

		// 1. 删除 instance
		// 只有真正删掉了行才写 DELETE 事件。sweeper 可能已经先一步清掉同一个过期实例，
		// 再写一次就是同一个 key 重复，还会白白推高全局 revision。
		deleted, err := r.store.Instance.Delete(ctx, instance.id)
		if err != nil {
			return errors.WithMessage(err, "delete instance failed")
		}
		if !deleted {
			slog.WarnContext(ctx, "unregister instance skipped, instance already deleted",
				slog.Int64("instance_id", instance.id),
				slog.String("instance_key", instance.key),
			)
			return nil
		}

		// 2. 写 instance event
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

		// 3. 更新全局 revision
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

// 关闭 registry，所有心跳协程停止并退出。
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
			// 走到这里 keepalive 的 ctx 大概率已经 cancelled，所以 unregister 另取一个新 ctx
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

		// sweeper 可能在前一刻先把 instance 删了，下一刻才轮到心跳更新，更新就会失败。
		// 这里进程并没有挂掉，只是行被 sweeper 清掉了，所以尝试自动重新注册
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

	oldId := instance.GetId()
	newInst, err := r.registerOnce(ctx, instance.group, instance.value)
	if err != nil {
		return errors.WithMessage(err, "register replacement instance failed")
	}

	instance.Replace(newInst)

	// Replace 之后 instance.id 已变成新实例的 id，locals 的键必须同步搬迁，
	// 否则后续 Unregister/Close 按新 id 查不到该实例，既不会删行也不会写 DELETE，
	// 留下的行只能等 sweeper 过期清理，同时 locals 还会残留旧 id 的条目。
	r.mu.Lock()
	if cInst, ok := r.locals[oldId]; ok {
		delete(r.locals, oldId)
		r.locals[newInst.id] = cInst
	}
	r.mu.Unlock()

	return nil
}
