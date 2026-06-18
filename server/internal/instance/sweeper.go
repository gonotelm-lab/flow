package instance

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"

	pkgerr "github.com/pkg/errors"
)

const (
	defaultSweepInterval = time.Second * 5
	defaultSweepBatch    = 200
	// 在基础间隔上做 ±20% 抖动，降低多实例同频扫描造成的尖峰压力。
	defaultSweepJitterRatio = 0.2
)

type Sweeper struct {
	txMgr repository.TxManager
	store repository.Store

	interval  time.Duration
	batchSize int

	closing atomic.Bool
	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewSweeper(
	txMgr repository.TxManager,
	store repository.Store,
) *Sweeper {
	return &Sweeper{
		txMgr:     txMgr,
		store:     store,
		interval:  defaultSweepInterval,
		batchSize: defaultSweepBatch,
	}
}

func (s *Sweeper) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing.Load() || s.cancel != nil {
		return
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.wg.Add(1)
	go s.loop(runCtx)
}

func (s *Sweeper) Close() {
	if !s.closing.CompareAndSwap(false, true) {
		return
	}

	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
}

// SweepOnce 执行一批过期实例清理。
// 返回值表示本次实际清理的实例数量。
func (s *Sweeper) SweepOnce(ctx context.Context) (int, error) {
	nowMs := time.Now().UnixMilli()
	expired, err := s.store.Instance.ListExpired(ctx, nowMs, s.batchSize)
	if err != nil {
		return 0, pkgerr.WithMessage(err, "list expired instances failed")
	}
	if len(expired) == 0 {
		return 0, nil
	}

	deletedCnt := 0
	err = s.txMgr.Transact(ctx, func(ctx context.Context) error {
		curRev, err := s.store.GlobalRevision.GetOrInitForUpdate(ctx, zeroRevision())
		if err != nil {
			return pkgerr.WithMessage(err, "get global revision failed")
		}
		revision := curRev.CurrentRevision

		for _, inst := range expired {
			deleted, err := s.store.Instance.DeleteExpired(ctx, inst.Id, nowMs)
			if err != nil {
				return pkgerr.WithMessage(err, "delete expired instance failed")
			}
			if !deleted {
				continue
			}

			revision++
			err = s.store.InstanceEvent.Append(ctx, &schema.InstanceEvent{
				Revision:   revision,
				Group:      inst.Group,
				Key:        inst.Key,
				Value:      inst.Value,
				Type:       InstanceEventDelete.String(),
				CreateTime: nowMs,
			})
			if err != nil {
				return pkgerr.WithMessage(err, "append instance event failed")
			}

			err = s.store.GlobalRevision.IncrRevision(ctx, discovRevisionName, nowMs)
			if err != nil {
				return pkgerr.WithMessage(err, "update global revision failed")
			}
			deletedCnt++
		}

		return nil
	})
	if err != nil {
		return 0, pkgerr.WithMessage(err, "sweep transaction failed")
	}

	return deletedCnt, nil
}

func (s *Sweeper) loop(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		s.cancel = nil
		s.mu.Unlock()
		s.wg.Done()
	}()

	timer := time.NewTimer(s.nextSweepInterval())
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "sweeper loop stopped")
			return
		case <-timer.C:
		}

		deleted, err := s.SweepOnce(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "sweep instances failed", slog.Any("err", err))
		} else if deleted > 0 {
			slog.InfoContext(ctx, "sweep instances done", slog.Int("deleted_count", deleted))
		}

		timer.Reset(s.nextSweepInterval())
	}
}

func (s *Sweeper) nextSweepInterval() time.Duration {
	base := s.interval
	if base <= 0 {
		base = defaultSweepInterval
	}

	jitterRange := time.Duration(float64(base) * defaultSweepJitterRatio)
	if jitterRange <= 0 {
		return base
	}

	// [base-jitterRange, base+jitterRange] 区间内随机取值。
	delta := time.Duration(rand.Int63n(int64(jitterRange)*2+1)) - jitterRange
	next := base + delta
	if next <= 0 {
		return time.Millisecond
	}

	return next
}
