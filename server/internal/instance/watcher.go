package instance

import (
	"context"
	stderr "errors"
	"log/slog"
	"time"

	pkgsql "github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"

	pkgerr "github.com/pkg/errors"
)

const (
	defaultWatchInterval        = time.Millisecond * 500
	defaultWatchBatchSize       = 200
	defaultWatchMaxRetryBackoff = time.Second * 10
)

// WatchCallback 外部可注入的事件回调。
// callback 按 revision 升序串行触发，返回 error 会终止 Watch。
type WatchCallback func(ctx context.Context, event *InstanceEvent) error

type Watcher struct {
	store repository.Store

	interval        time.Duration
	batchSize       int
	maxRetryBackoff time.Duration
}

func NewWatcher(store repository.Store) *Watcher {
	return &Watcher{
		store:           store,
		interval:        defaultWatchInterval,
		batchSize:       defaultWatchBatchSize,
		maxRetryBackoff: defaultWatchMaxRetryBackoff,
	}
}

// Watch 从当前 global revision 开始 watch（不回放历史）。
func (w *Watcher) Watch(
	ctx context.Context,
	group string,
	callback WatchCallback,
) error {
	lastRevision, err := w.CurrentRevision(ctx)
	if err != nil {
		return pkgerr.WithMessage(err, "get current revision failed")
	}

	return w.WatchWithRevision(ctx, group, lastRevision, callback)
}

// CurrentRevision 返回当前 discovery 的 revision 水位。
func (w *Watcher) CurrentRevision(ctx context.Context) (int64, error) {
	rev, err := w.store.GlobalRevision.Get(ctx, discovRevisionName)
	if err != nil {
		if stderr.Is(err, pkgsql.ErrNoRecord) {
			return 0, nil
		}

		return 0, pkgerr.WithMessage(err, "get global revision failed")
	}

	return rev.CurrentRevision, nil
}

// WatchWithRevision 按 revision 增量轮询并触发回调。
// - group: 事件分组（如 flow/instances）
// - lastRevision: 已消费到的 revision；仅消费 > lastRevision 的新事件
func (w *Watcher) WatchWithRevision(
	ctx context.Context,
	group string,
	lastRevision int64,
	callback WatchCallback,
) error {
	if callback == nil {
		return pkgerr.New("watch callback is nil")
	}
	if group == "" {
		return pkgerr.New("watch group is empty")
	}
	if lastRevision < 0 {
		return pkgerr.New("lastRevision must be non-negative")
	}

	backoff := w.interval
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		events, err := w.store.InstanceEvent.List(ctx, group, lastRevision, w.batchSize)
		if err != nil {
			slog.WarnContext(ctx, "watch list events failed",
				slog.String("group", group),
				slog.Int64("last_revision", lastRevision),
				slog.Any("err", err),
			)
			if err := sleepContext(ctx, backoff); err != nil {
				return nil
			}
			backoff = growBackoff(backoff, w.maxRetryBackoff)
			continue
		}

		// 查询成功后，回退到基础轮询间隔。
		backoff = w.interval
		if len(events) == 0 {
			if err := sleepContext(ctx, w.interval); err != nil {
				return nil
			}
			continue
		}

		for _, raw := range events {
			event := fromSchemaInstanceEvent(raw)
			if err := callback(ctx, event); err != nil {
				return pkgerr.WithMessage(err, "watch callback failed")
			}
			lastRevision = raw.Revision
		}
	}
}

func fromSchemaInstanceEvent(event *schema.InstanceEvent) *InstanceEvent {
	return &InstanceEvent{
		Revision:   event.Revision,
		Group:      event.Group,
		Key:        event.Key,
		Value:      event.Value,
		EventType:  InstanceEventType(event.Type),
		CreateTime: event.CreateTime,
	}
}

func growBackoff(current time.Duration, max time.Duration) time.Duration {
	if current <= 0 {
		current = defaultWatchInterval
	}
	if max <= 0 {
		max = defaultWatchMaxRetryBackoff
	}

	next := current * 2
	if next > max {
		return max
	}
	return next
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
