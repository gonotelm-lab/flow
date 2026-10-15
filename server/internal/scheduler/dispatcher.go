package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/internal/sharding"
	"github.com/google/uuid"
)

// 投递阶段。
const (
	phaseFast       = schema.AttemptPhaseFast
	phaseDeadLetter = schema.AttemptPhaseDeadLetter
)

// 单次落后对齐时，skipped_count 最多累加多少。避免一次超长停机后把计数算爆。
const maxCountedSkipped = 1000

// ShardProvider 返回本实例当前负责的分片区间。
//
// 它由实例注册表推导（app.selfShard），会随扩缩容变化。分片只决定「谁有资格扫」，
// 不决定「谁真的执行」——后者由仓储层的认领与租约保证。
type ShardProvider func() *sharding.Shard

type Dispatcher struct {
	store   *repository.Store
	cfg     *config.SchedulerConfig
	shardFn ShardProvider

	// owner 是本实例在租约里的标识（实例注册表的 key），用于排查「这个 fire 归谁」。
	owner string

	httpClient *http.Client

	// sem 限制单实例的并发投递数
	sem chan struct{}

	wg sync.WaitGroup

	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewDispatcher(
	repoStore *repository.Store,
	cfg *config.SchedulerConfig,
	shardFn ShardProvider,
	owner string,
) *Dispatcher {
	return &Dispatcher{
		store:   repoStore,
		cfg:     cfg,
		shardFn: shardFn,
		owner:   owner,
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
			// 明确不跟随重定向：回调语义要求「打给这个地址」，跟着跳转会让目标脱离
			// 注册方的控制，也会绕过我们对 URL 的校验边界。
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		sem: make(chan struct{}, cfg.DeliverConcurrency),
	}
}

// Run 阻塞运行调度循环，直到 ctx 结束或 Close 被调用。
func (d *Dispatcher) Run(ctx context.Context) {
	d.wg.Add(1)
	defer d.wg.Done()

	runCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()
	defer func() {
		cancel()
		d.mu.Lock()
		d.cancel = nil
		d.mu.Unlock()
	}()

	defer func() {
		if e := recover(); e != nil {
			slog.ErrorContext(runCtx, "scheduler dispatcher panic",
				slog.Any("err", e),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()

	ticker := time.NewTicker(d.cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-runCtx.Done():
			return
		case <-ticker.C:
			d.tick(runCtx)
		}
	}
}

// Close 停止调度循环，并等待所有在途投递结束。
func (d *Dispatcher) Close() {
	d.mu.Lock()
	cancel := d.cancel
	d.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	d.wg.Wait()
}

func (d *Dispatcher) tick(ctx context.Context) {
	shard := d.shardFn()
	// 无业可分（实例数超过分片数，SequentialCalculator 会算出空区间）时纯待命。
	if shard == nil || !shard.Valid() || shard.Start > shard.End {
		return
	}

	now, err := d.store.Schedule.Now(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "scheduler get db time failed", slog.Any("err", err))
		return
	}

	claimParams := &store.ScheduleClaimParams{
		ShardStart: shard.Start,
		ShardEnd:   shard.End,
		Now:        now,
		BatchSize:  d.cfg.ScanBatch,
		LeaseOwner: d.leaseOwner(),
		LeaseUntil: now + d.cfg.LeaseDuration.Milliseconds(),
	}

	due, err := d.store.Schedule.ClaimDue(ctx, claimParams)
	if err != nil {
		slog.ErrorContext(ctx, "scheduler claim due failed", slog.Any("err", err))
	} else {
		d.dispatch(ctx, due, now)
	}

	retries, err := d.store.Schedule.ClaimRetry(ctx, claimParams)
	if err != nil {
		slog.ErrorContext(ctx, "scheduler claim retry failed", slog.Any("err", err))
	} else {
		d.dispatch(ctx, retries, now)
	}
}

func (d *Dispatcher) dispatch(ctx context.Context, rows []*schema.Schedule, claimedAt int64) {
	for _, row := range rows {
		if row.FireId == nil {
			continue
		}

		// 在循环里先占信号量：这样在途 goroutine 数被 DeliverConcurrency 限住，
		// 不会因为一批认领 100 条就一次性开 100 个连接。
		d.sem <- struct{}{}
		d.wg.Add(1)

		go func(s *schema.Schedule) {
			defer d.wg.Done()
			defer func() { <-d.sem }()
			defer func() {
				if e := recover(); e != nil {
					slog.ErrorContext(ctx, "scheduler delivery panic",
						slog.Any("err", e),
						slog.String("schedule_id", s.Id.String()),
						slog.String("stack", string(debug.Stack())),
					)
				}
			}()

			d.attempt(ctx, s, claimedAt)
		}(row)
	}
}

// attempt 完成「一次」投递：可能是一次真正的 HTTP 调用，也可能是一次迟到跳过。
func (d *Dispatcher) attempt(ctx context.Context, s *schema.Schedule, claimedAt int64) {
	fireId := *s.FireId
	fresh := s.FireState != nil && *s.FireState == schema.FireStatePending

	// 迟到跳过只针对「刚认领、从未投递过」的 cron occurrence。
	//
	// 迟到的成因只有一种：这个分片有一段时间没人扫（持有它的 pod 挂了、或注册表还没
	// 反应过来）。按「不补发」的决定，这种 occurrence 直接丢弃并重算，而不是迟到地打
	// 一次——用户写 0 * * * * * 是要整点，不是要「恢复后立刻」。
	//
	// 注意 fire_scheduled_time == 0 是 TriggerNow 的标记：手动触发没有 occurrence 概念，
	// 不参与迟到判定，也不推进 next_fire_at。
	if fresh && s.ScheduleType == schema.ScheduleTypeCron && s.FireScheduledTime > 0 {
		tolerance := d.cfg.LatenessTolerance.Milliseconds()
		if claimedAt-s.FireScheduledTime > tolerance {
			d.skipLate(ctx, s, fireId, claimedAt)
			return
		}
	}

	phase := phaseFast
	if s.FireState != nil && *s.FireState == schema.FireStateDeadLetter {
		phase = phaseDeadLetter
	}

	if phase == phaseDeadLetter {
		d.attemptDeadLetter(ctx, s, fireId, claimedAt)
		return
	}

	// 快速阶段：第 attemptNo 次尝试（首次为 1）
	attemptNo := s.AttemptNo
	if attemptNo <= 0 {
		attemptNo = 1
	}

	result := d.deliver(ctx, s)
	if result.ok {
		d.finishSuccess(ctx, s, fireId, claimedAt)
		return
	}

	d.recordFailure(ctx, s, fireId, phase, attemptNo, result, claimedAt)

	maxFast := len(d.cfg.FastRetryIntervals) + 1
	if attemptNo >= maxFast {
		// 快速重试耗尽：转入死信，改用慢节奏继续尝试。
		d.enterDeadLetter(ctx, s, fireId, claimedAt, result)
		return
	}

	nextAt := claimedAt + jittered(d.cfg.FastRetryIntervals[attemptNo-1], d.cfg.RetryJitterRatio).Milliseconds()
	delivering := schema.FireStateDelivering

	d.apply(ctx, s, fireId, &store.AttemptOutcome{
		NextFireState: &delivering,
		NextAttemptAt: nextAt,
		AttemptNo:     attemptNo + 1,
		LastError:     result.errText,
		Now:           claimedAt,
	})
}

// attemptDeadLetter 执行一次死信慢重试。
func (d *Dispatcher) attemptDeadLetter(
	ctx context.Context,
	s *schema.Schedule,
	fireId uuid.UUID,
	claimedAt int64,
) {
	dl, err := d.store.ScheduleDeadLetter.GetRetryingBySchedule(ctx, s.Id)
	if err != nil {
		// 死信记录不见了（可能被人工 Discard）：释放槽位，避免 schedule 永久卡住。
		slog.WarnContext(ctx, "retrying dead letter not found, releasing fire slot",
			slog.String("schedule_id", s.Id.String()),
			slog.Any("err", err),
		)
		d.releaseSlot(ctx, s, fireId, claimedAt)
		return
	}

	attemptNo := int(dl.AttemptCount) + 1
	result := d.deliver(ctx, s)
	d.recordFailure(ctx, s, fireId, phaseDeadLetter, attemptNo, result, claimedAt)

	if result.ok {
		if _, err := d.store.ScheduleDeadLetter.SetState(
			ctx, dl.Id, schema.DeadLetterStateRetrying, schema.DeadLetterStateReplayed, claimedAt,
		); err != nil {
			slog.ErrorContext(ctx, "mark dead letter replayed failed", slog.Any("err", err))
		}
		d.finishSuccess(ctx, s, fireId, claimedAt)
		return
	}

	if attemptNo >= len(d.cfg.DeadLetterIntervals) {
		// 慢重试也耗尽：死信转 EXHAUSTED（不阻塞后续触发，等人工 Replay / Discard）。
		if _, err := d.store.ScheduleDeadLetter.SetState(
			ctx, dl.Id, schema.DeadLetterStateRetrying, schema.DeadLetterStateExhausted, claimedAt,
		); err != nil {
			slog.ErrorContext(ctx, "mark dead letter exhausted failed", slog.Any("err", err))
		}

		newState := ""
		if s.ScheduleType == schema.ScheduleTypeOneShot {
			newState = schema.ScheduleStateFailed
		}
		d.apply(ctx, s, fireId, &store.AttemptOutcome{
			NextFireState: nil,
			NextFireAt:    d.nextFireAtAfter(s, claimedAt),
			NewState:      newState,
			LastError:     result.errText,
			Now:           claimedAt,
		})
		return
	}

	nextAt := claimedAt + jittered(d.cfg.DeadLetterIntervals[attemptNo], d.cfg.RetryJitterRatio).Milliseconds()

	if _, err := d.store.ScheduleDeadLetter.UpdateRetry(ctx, dl.Id, &store.DeadLetterRetryParams{
		AttemptCount:   attemptNo,
		NextAttemptAt:  nextAt,
		LastHttpStatus: result.httpStatus,
		LastError:      result.errText,
		UpdateTime:     claimedAt,
	}); err != nil {
		slog.ErrorContext(ctx, "update dead letter retry failed", slog.Any("err", err))
	}

	deadLetterState := schema.FireStateDeadLetter
	d.apply(ctx, s, fireId, &store.AttemptOutcome{
		NextFireState: &deadLetterState,
		NextAttemptAt: nextAt,
		AttemptNo:     s.AttemptNo + 1,
		LastError:     result.errText,
		Now:           claimedAt,
	})
}

// skipLate 丢弃一个迟到的 occurrence：释放槽位、把 next_fire_at 对齐到当前时间之后，
// 并把错过的次数计入 skipped_count。
func (d *Dispatcher) skipLate(
	ctx context.Context,
	s *schema.Schedule,
	fireId uuid.UUID,
	now int64,
) {
	next, err := NextMillis(s.CronExpr, s.Timezone, now)
	if err != nil {
		slog.ErrorContext(ctx, "skip late: compute next fire time failed",
			slog.String("schedule_id", s.Id.String()),
			slog.Any("err", err),
		)
		d.releaseSlot(ctx, s, fireId, now)
		return
	}

	missed := 0
	if count, cerr := CountMissedMillis(
		s.CronExpr, s.Timezone, s.FireScheduledTime, now, maxCountedSkipped,
	); cerr == nil {
		missed = count
	}

	if _, err := d.store.Schedule.SkipDue(ctx, s.Id, fireId, next, int64(missed), now); err != nil {
		slog.ErrorContext(ctx, "skip late occurrence failed",
			slog.String("schedule_id", s.Id.String()),
			slog.Any("err", err),
		)
		return
	}

	slog.InfoContext(ctx, "skipped late occurrence (no catch-up by design)",
		slog.String("schedule_id", s.Id.String()),
		slog.Int64("scheduled_time", s.FireScheduledTime),
		slog.Int64("late_by_ms", now-s.FireScheduledTime),
		slog.Int("missed", missed),
	)
}

// finishSuccess 投递成功：释放槽位、更新 last_success_at、推进 next_fire_at。
func (d *Dispatcher) finishSuccess(
	ctx context.Context,
	s *schema.Schedule,
	fireId uuid.UUID,
	now int64,
) {
	newState := ""
	if s.ScheduleType == schema.ScheduleTypeOneShot {
		newState = schema.ScheduleStateFinished
	}
	d.apply(ctx, s, fireId, &store.AttemptOutcome{
		NextFireState: nil,
		NextFireAt:    d.nextFireAtAfter(s, now),
		NewState:      newState,
		Success:       true,
		Now:           now,
	})
}

// releaseSlot 仅释放槽位，不推进 next_fire_at（异常兜底路径）。
func (d *Dispatcher) releaseSlot(ctx context.Context, s *schema.Schedule, fireId uuid.UUID, now int64) {
	d.apply(ctx, s, fireId, &store.AttemptOutcome{
		NextFireState: nil,
		Now:           now,
	})
}

func (d *Dispatcher) apply(
	ctx context.Context,
	s *schema.Schedule,
	fireId uuid.UUID,
	outcome *store.AttemptOutcome,
) {
	if _, err := d.store.Schedule.ApplyOutcome(ctx, s.Id, fireId, outcome); err != nil {
		slog.ErrorContext(ctx, "apply schedule outcome failed",
			slog.String("schedule_id", s.Id.String()),
			slog.Any("err", err),
		)
	}
}

// nextFireAtAfter 算出本次 fire 结束后的 next_fire_at。
//
//   - 手动触发（fire_scheduled_time == 0）不动 next_fire_at，也就不会打乱 cron 的自然节奏；
//   - cron 先从被消费掉的 occurrence 往后推一次；如果已经落后（本次投递连同重试耗时过长），
//     直接对齐到当前时间之后，并把期间错过的次数结算进 skipped_count——这就是「不补发」；
//   - 一次性任务返回 0（不修改），它的终态由 state 表达。
func (d *Dispatcher) nextFireAtAfter(s *schema.Schedule, now int64) int64 {
	if s.ScheduleType != schema.ScheduleTypeCron || s.FireScheduledTime <= 0 {
		return 0
	}

	next, err := NextMillis(s.CronExpr, s.Timezone, s.FireScheduledTime)
	if err != nil {
		slog.Error("compute next fire time failed",
			slog.String("schedule_id", s.Id.String()),
			slog.Any("err", err),
		)
		return 0
	}
	if next > now {
		return next
	}

	aligned, err := NextMillis(s.CronExpr, s.Timezone, now)
	if err != nil {
		return 0
	}
	return aligned
}

func (d *Dispatcher) enterDeadLetter(
	ctx context.Context,
	s *schema.Schedule,
	fireId uuid.UUID,
	now int64,
	result deliveryResult,
) {
	dlId, err := uuid.NewV7()
	if err != nil {
		slog.ErrorContext(ctx, "generate dead letter id failed", slog.Any("err", err))
		d.releaseSlot(ctx, s, fireId, now)
		return
	}

	nextAt := now + jittered(d.cfg.DeadLetterIntervals[0], d.cfg.RetryJitterRatio).Milliseconds()

	if _, err := d.store.ScheduleDeadLetter.Create(ctx, &schema.ScheduleDeadLetter{
		Id:             dlId,
		ScheduleId:     s.Id,
		Namespace:      s.Namespace,
		FireId:         fireId,
		ScheduledTime:  s.FireScheduledTime,
		AttemptCount:   0,
		State:          schema.DeadLetterStateRetrying,
		NextAttemptAt:  nextAt,
		LastHttpStatus: result.httpStatus,
		LastError:      result.errText,
		CreateTime:     now,
		UpdateTime:     now,
	}); err != nil {
		slog.ErrorContext(ctx, "create dead letter failed",
			slog.String("schedule_id", s.Id.String()),
			slog.Any("err", err),
		)
		d.releaseSlot(ctx, s, fireId, now)
		return
	}

	deadLetterState := schema.FireStateDeadLetter
	d.apply(ctx, s, fireId, &store.AttemptOutcome{
		NextFireState: &deadLetterState,
		NextAttemptAt: nextAt,
		AttemptNo:     s.AttemptNo + 1,
		LastError:     result.errText,
		Now:           now,
	})
}

// recordFailure 每个失败的尝试落一行；成功不记录。
func (d *Dispatcher) recordFailure(
	ctx context.Context,
	s *schema.Schedule,
	fireId uuid.UUID,
	phase string,
	attemptNo int,
	result deliveryResult,
	now int64,
) {
	if err := d.store.ScheduleAttemptFailure.Create(ctx, &schema.ScheduleAttemptFailure{
		ScheduleId:    s.Id,
		Namespace:     s.Namespace,
		FireId:        fireId,
		AttemptNo:     attemptNo,
		Phase:         phase,
		ScheduledTime: s.FireScheduledTime,
		AttemptTime:   now,
		HttpStatus:    result.httpStatus,
		DurationMs:    result.durationMs,
		Error:         result.errText,
		CreateTime:    now,
	}); err != nil {
		slog.ErrorContext(ctx, "record attempt failure failed",
			slog.String("schedule_id", s.Id.String()),
			slog.Any("err", err),
		)
	}
}

type deliveryResult struct {
	ok         bool
	httpStatus int
	durationMs int64
	errText    string
}

// deliver 发起一次回调。
//
// 成功判定：HTTP 2xx。其余（3xx 因为不跟随重定向、以及 4xx/5xx）与超时、连接失败
// 一律算失败。
func (d *Dispatcher) deliver(ctx context.Context, s *schema.Schedule) deliveryResult {
	secret, err := d.namespaceKey(ctx, s.Namespace)
	if err != nil {
		return deliveryResult{errText: "failed to load namespace api key: " + err.Error()}
	}

	start := time.Now()

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, s.CallbackUrl, bytes.NewReader(s.Payload))
	if err != nil {
		return deliveryResult{errText: "failed to build request: " + err.Error()}
	}

	req.Header.Set("Content-Type", s.ContentType)
	req.Header.Set(HeaderScheduleId, s.Id.String())
	if s.FireId != nil {
		req.Header.Set(HeaderFireId, s.FireId.String())
	}
	req.Header.Set(HeaderScheduledTime, strconv.FormatInt(s.FireScheduledTime, 10))

	// 时间戳参与签名，接收方据此判重放。
	ts := time.Now().Unix()
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(HeaderSignature, SignatureHeader(secret, ts, s.Payload))

	resp, err := d.httpClient.Do(req)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return deliveryResult{
			durationMs: duration,
			errText:    "http request failed: " + err.Error(),
		}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return deliveryResult{ok: true, httpStatus: resp.StatusCode, durationMs: duration}
	}

	// 只读一小段用于报错，避免把对方的大响应体读进来。
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return deliveryResult{
		httpStatus: resp.StatusCode,
		durationMs: duration,
		errText: fmt.Sprintf("unexpected status %d: %s",
			resp.StatusCode, truncateText(string(body), 256)),
	}
}

// namespaceKey 取签名密钥（namespace 的 api_key）。
//
// 每次投递现取而不是缓存：密钥是同一 namespace 下所有 schedule 共用的，缓存会引入
// 「改了密钥但旧值还在用」的窗口，而这里的查询量级（一次回调一次）完全可以接受。
func (d *Dispatcher) namespaceKey(ctx context.Context, namespace string) (string, error) {
	ns, err := d.store.Namespace.Get(ctx, namespace)
	if err != nil {
		return "", err
	}
	return ns.ApiKey, nil
}

func (d *Dispatcher) leaseOwner() string {
	return d.owner
}

func truncateText(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}

// jittered 给退避间隔加 ±ratio 的随机抖动。
//
// 必须有抖动：N 条 schedule 若都在整点触发并同时失败，没有抖动就会在同一毫秒一起重试，
// 自己打自己。
func jittered(base time.Duration, ratio float64) time.Duration {
	if ratio <= 0 || base <= 0 {
		return base
	}

	delta := float64(base) * ratio
	return time.Duration(float64(base) - delta + rand.Float64()*2*delta)
}
