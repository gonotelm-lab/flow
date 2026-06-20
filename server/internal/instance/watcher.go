package instance

import (
	"context"
	stderr "errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	pkgsql "github.com/gonotelm-lab/flow/server/pkg/sql"

	"github.com/pkg/errors"
)

const (
	defaultWatchInterval        = time.Millisecond * 500
	defaultWatchBatchSize       = 200
	defaultWatchMaxRetryBackoff = time.Second * 10
)

// WatchChan 对齐 etcd 风格：watch 返回只读 channel。
type WatchChan <-chan WatchResponse

// WatchResponse 对齐 etcd 风格：事件批次 + 终止状态。
type WatchResponse struct {
	Events   []*InstanceEvent
	Revision int64
	Canceled bool

	err error
}

func (r WatchResponse) Err() error {
	return r.err
}

type Watcher struct {
	store *repository.Store
	cfg   WatcherConfig
}

type WatcherConfig struct {
	Interval        time.Duration
	BatchSize       int
	MaxRetryBackoff time.Duration
}

func (cfg *WatcherConfig) Normalize() {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultWatchInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultWatchBatchSize
	}
	if cfg.MaxRetryBackoff <= 0 {
		cfg.MaxRetryBackoff = defaultWatchMaxRetryBackoff
	}
}

func NewWatcher(store *repository.Store, cfg WatcherConfig) *Watcher {
	cfg.Normalize()

	return &Watcher{
		store: store,
		cfg:   cfg,
	}
}

// Watch 从当前 global revision 开始 watch（不回放历史）。
func (w *Watcher) Watch(
	ctx context.Context,
	group string,
) WatchChan {
	ch := make(chan WatchResponse, 1)

	go func() {
		defer close(ch)

		lastRevision, err := w.CurrentRevision(ctx)
		if err != nil {
			w.sendWatchResponse(
				ctx,
				ch,
				newWatchErrorResponse(errors.WithMessage(err, "get current revision failed")),
			)
			return
		}

		w.watchLoop(ctx, group, lastRevision, ch)
	}()

	return ch
}

// CurrentRevision 返回当前 discovery 的 revision 水位。
func (w *Watcher) CurrentRevision(ctx context.Context) (int64, error) {
	if w.store == nil || w.store.GlobalRevision == nil {
		return 0, errors.New("global revision store is required")
	}

	rev, err := w.store.GlobalRevision.Get(ctx, discovRevisionName)
	if err != nil {
		if stderr.Is(err, pkgsql.ErrNoRecord) {
			return 0, nil
		}

		return 0, errors.WithMessage(err, "get global revision failed")
	}

	return rev.CurrentRevision, nil
}

// WatchWithRevision 按 revision 增量轮询并产出事件批次。
// - group: 事件分组（如 flow/instances）
// - lastRevision: 已消费到的 revision；仅消费 > lastRevision 的新事件
func (w *Watcher) WatchWithRevision(
	ctx context.Context,
	group string,
	lastRevision int64,
) WatchChan {
	ch := make(chan WatchResponse, 1)

	go func() {
		defer close(ch)
		w.watchLoop(ctx, group, lastRevision, ch)
	}()

	return ch
}

func (w *Watcher) watchLoop(
	ctx context.Context,
	group string,
	lastRevision int64,
	ch chan<- WatchResponse,
) {
	if err := w.validateWatchArgs(group, lastRevision); err != nil {
		w.sendWatchResponse(ctx, ch, newWatchErrorResponse(err))
		return
	}
	if w.store == nil || w.store.InstanceEvent == nil {
		w.sendWatchResponse(ctx, ch, newWatchErrorResponse(errors.New("instance event store is required")))
		return
	}

	backoff := w.cfg.Interval
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		events, err := w.store.InstanceEvent.List(ctx, group, lastRevision, w.cfg.BatchSize)
		if err != nil {
			slog.WarnContext(ctx, "watch list events failed",
				slog.String("group", group),
				slog.Int64("last_revision", lastRevision),
				slog.Any("err", err),
			)
			if err := sleepContext(ctx, backoff); err != nil {
				return
			}
			backoff = growBackoff(backoff, w.cfg.MaxRetryBackoff)
			continue
		}

		// 查询成功后，回退到基础轮询间隔。
		backoff = w.cfg.Interval
		if len(events) == 0 {
			if err := sleepContext(ctx, w.cfg.Interval); err != nil {
				return
			}
			continue
		}

		respEvents := make([]*InstanceEvent, 0, len(events))
		currentRevision := lastRevision

		for _, raw := range events {
			if raw == nil {
				continue
			}

			respEvents = append(respEvents, fromSchemaInstanceEvent(raw))
			currentRevision = raw.Revision
		}
		if len(respEvents) == 0 {
			continue
		}

		if ok := w.sendWatchResponse(ctx, ch, WatchResponse{
			Events:   respEvents,
			Revision: currentRevision,
		}); !ok {
			return
		}
		lastRevision = currentRevision
	}
}

func (w *Watcher) validateWatchArgs(group string, lastRevision int64) error {
	if strings.TrimSpace(group) == "" {
		return errors.New("group must not be empty")
	}
	if lastRevision < 0 {
		return errors.New("lastRevision must be non-negative")
	}
	return nil
}

func (w *Watcher) sendWatchResponse(
	ctx context.Context,
	ch chan<- WatchResponse,
	resp WatchResponse,
) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- resp:
		return true
	}
}

func newWatchErrorResponse(err error) WatchResponse {
	return WatchResponse{
		Canceled: true,
		err:      err,
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
