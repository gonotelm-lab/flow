package postgres

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/impl/util"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ScheduleStoreImpl struct {
	db *gorm.DB
}

func NewScheduleStoreImpl(db *gorm.DB) store.Schedule {
	return &ScheduleStoreImpl{db: db}
}

func (s *ScheduleStoreImpl) Create(
	ctx context.Context,
	sched *schema.Schedule,
) (*schema.Schedule, error) {
	db := util.GetDB(ctx, s.db)
	if err := db.Create(sched).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return sched, nil
}

func (s *ScheduleStoreImpl) Get(ctx context.Context, id uuid.UUID) (*schema.Schedule, error) {
	db := util.GetDB(ctx, s.db)
	var sched schema.Schedule
	if err := db.Where("id = ?", id).First(&sched).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return &sched, nil
}

func (s *ScheduleStoreImpl) GetByIdempotencyKey(
	ctx context.Context,
	namespace, key string,
) (*schema.Schedule, error) {
	db := util.GetDB(ctx, s.db)
	var sched schema.Schedule
	if err := db.Where("namespace = ? AND idempotency_key = ?", namespace, key).
		First(&sched).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return &sched, nil
}

// Now 返回数据库当前时间（unix 毫秒），作为全集群的统一时钟源。
func (s *ScheduleStoreImpl) Now(ctx context.Context) (int64, error) {
	db := util.GetDB(ctx, s.db)
	var now int64
	if err := db.Raw("SELECT (EXTRACT(EPOCH FROM now()) * 1000)::BIGINT").Scan(&now).Error; err != nil {
		return 0, sql.WrapError(err)
	}
	return now, nil
}

func (s *ScheduleStoreImpl) List(
	ctx context.Context,
	params *store.ScheduleListParams,
) ([]*schema.Schedule, int64, error) {
	db := util.GetDB(ctx, s.db)
	q := db.Model(&schema.Schedule{})

	if params.Namespace != "" {
		q = q.Where("namespace = ?", params.Namespace)
	}
	if params.ScheduleType != "" {
		q = q.Where("schedule_type = ?", params.ScheduleType)
	}
	if params.State != "" {
		q = q.Where("state = ?", params.State)
	}
	if params.Id != "" {
		q = q.Where("id::text ILIKE ?", "%"+params.Id+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	var list []*schema.Schedule
	if err := q.Offset(params.Offset).Limit(params.Limit).
		Order("create_time DESC").
		Find(&list).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	return list, total, nil
}

// ClaimDue 认领本分片范围内到期的 occurrence。
//
// 关键点：SELECT ... FOR UPDATE SKIP LOCKED 与随后的 CAS UPDATE 在同一个事务里。
// 两个实例同时扫到同一行时，只有一个能拿到行锁，另一个直接跳过（不阻塞）；提交后
// fire_state 非空，该行立刻从 idx_schedules_due 这个部分索引里消失，别的实例再也扫不到。
//
// 这是「绝不重复投递」的唯一落点——分片归属只决定谁有资格扫，claim 才决定谁真的执行。
func (s *ScheduleStoreImpl) ClaimDue(
	ctx context.Context,
	p *store.ScheduleClaimParams,
) ([]*schema.Schedule, error) {
	db := util.GetDB(ctx, s.db)
	claimed := make([]*schema.Schedule, 0, p.BatchSize)

	err := db.Transaction(func(tx *gorm.DB) error {
		var rows []*schema.Schedule
		err := tx.Clauses(clause.Locking{
			Strength: clause.LockingStrengthUpdate,
			Options:  clause.LockingOptionsSkipLocked,
		}).
			Where("shard BETWEEN ? AND ?", p.ShardStart, p.ShardEnd).
			Where("state = ?", schema.ScheduleStateActive).
			Where("fire_state IS NULL").
			Where("next_fire_at <= ?", p.Now).
			Order("shard ASC, next_fire_at ASC").
			Limit(p.BatchSize).
			Find(&rows).Error
		if err != nil {
			return err
		}

		for _, row := range rows {
			fireId, idErr := uuid.NewV7()
			if idErr != nil {
				return idErr
			}

			res := tx.Model(&schema.Schedule{}).
				Where("id = ? AND fire_state IS NULL", row.Id).
				Updates(map[string]any{
					"fire_id":             fireId,
					"fire_scheduled_time": row.NextFireAt,
					"fire_state":          schema.FireStatePending,
					"attempt_no":          1,
					"next_attempt_at":     p.Now,
					"lease_owner":         p.LeaseOwner,
					"lease_until":         p.LeaseUntil,
					"update_time":         p.Now,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				continue
			}

			row.FireId = &fireId
			row.FireScheduledTime = row.NextFireAt
			fireState := schema.FireStatePending
			row.FireState = &fireState
			row.AttemptNo = 1
			row.NextAttemptAt = p.Now
			row.LeaseOwner = p.LeaseOwner
			row.LeaseUntil = p.LeaseUntil
			row.UpdateTime = p.Now
			claimed = append(claimed, row)
		}

		return nil
	})
	if err != nil {
		return nil, sql.WrapError(err)
	}

	return claimed, nil
}

// ClaimRetry 认领需要继续投递的 fire：快重试、死信慢重试，以及租约过期后的接管。
//
// 租约过期前不会被任何实例（包括自己）重新认领，所以正常投递期间不存在重复投递窗口；
// 只有持有租约的实例崩溃、租约过期后才会被接管重投——这就是 at-least-once 的唯一来源，
// FireId 在此期间保持不变，接收方据此去重。
func (s *ScheduleStoreImpl) ClaimRetry(
	ctx context.Context,
	p *store.ScheduleClaimParams,
) ([]*schema.Schedule, error) {
	db := util.GetDB(ctx, s.db)
	claimed := make([]*schema.Schedule, 0, p.BatchSize)

	err := db.Transaction(func(tx *gorm.DB) error {
		var rows []*schema.Schedule
		err := tx.Clauses(clause.Locking{
			Strength: clause.LockingStrengthUpdate,
			Options:  clause.LockingOptionsSkipLocked,
		}).
			Where("shard BETWEEN ? AND ?", p.ShardStart, p.ShardEnd).
			Where("fire_state IS NOT NULL").
			Where("next_attempt_at <= ?", p.Now).
			Where("lease_until <= ?", p.Now).
			Order("shard ASC, next_attempt_at ASC").
			Limit(p.BatchSize).
			Find(&rows).Error
		if err != nil {
			return err
		}

		for _, row := range rows {
			// PENDING -> DELIVERING：已经被认领过但还没投递的 fire（例如认领后进程就崩了），
			// 再次被认领时必须变成「已投递过」的语义，调度器才不会再拿迟到容差去跳过它
			// ——否则这次触发会被静默丢弃。
			//
			// DEAD_LETTER 保持不变：它是死信慢重试阶段，调度器靠它决定用哪套退避节奏。
			res := tx.Model(&schema.Schedule{}).
				Where("id = ? AND fire_state IS NOT NULL AND lease_until <= ?", row.Id, p.Now).
				Updates(map[string]any{
					"fire_state": gorm.Expr(
						"CASE WHEN fire_state = ? THEN ? ELSE fire_state END",
						schema.FireStatePending, schema.FireStateDelivering),
					"lease_owner": p.LeaseOwner,
					"lease_until": p.LeaseUntil,
					"update_time": p.Now,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				continue
			}

			if row.FireState != nil && *row.FireState == schema.FireStatePending {
				delivering := schema.FireStateDelivering
				row.FireState = &delivering
			}
			row.LeaseOwner = p.LeaseOwner
			row.LeaseUntil = p.LeaseUntil
			row.UpdateTime = p.Now
			claimed = append(claimed, row)
		}

		return nil
	})
	if err != nil {
		return nil, sql.WrapError(err)
	}

	return claimed, nil
}

func (s *ScheduleStoreImpl) ClaimByID(
	ctx context.Context,
	id uuid.UUID,
	p *store.ClaimByIDParams,
) (bool, error) {
	db := util.GetDB(ctx, s.db)
	res := db.Model(&schema.Schedule{}).
		Where("id = ? AND fire_state IS NULL", id).
		Updates(map[string]any{
			"fire_id":             p.FireId,
			"fire_scheduled_time": p.FireScheduledTime,
			"fire_state":          p.FireState,
			"attempt_no":          1,
			"next_attempt_at":     p.Now,
			"lease_owner":         p.LeaseOwner,
			"lease_until":         p.LeaseUntil,
			"update_time":         p.Now,
		})
	if res.Error != nil {
		return false, sql.WrapError(res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (s *ScheduleStoreImpl) SkipDue(
	ctx context.Context,
	id, fireId uuid.UUID,
	nextFireAt, skipped, now int64,
) (bool, error) {
	db := util.GetDB(ctx, s.db)
	res := db.Model(&schema.Schedule{}).
		Where("id = ? AND fire_id = ? AND fire_state IS NOT NULL", id, fireId).
		Updates(map[string]any{
			"fire_id":         nil,
			"fire_state":      nil,
			"attempt_no":      0,
			"next_attempt_at": int64(0),
			"lease_owner":     "",
			"lease_until":     int64(0),
			"next_fire_at":    nextFireAt,
			"skipped_count":   gorm.Expr("skipped_count + ?", skipped),
			"update_time":     now,
		})
	if res.Error != nil {
		return false, sql.WrapError(res.Error)
	}
	return res.RowsAffected > 0, nil
}

// ApplyOutcome 落地一次投递尝试的结果。
//
// NextFireState 为 nil 表示释放槽位：该 schedule 恢复可触发。此时若 NextFireAt > 0
// 则同时推进 next_fire_at，保证「被消费掉的 occurrence 不会再被触发」。
func (s *ScheduleStoreImpl) ApplyOutcome(
	ctx context.Context,
	id, fireId uuid.UUID,
	p *store.AttemptOutcome,
) (bool, error) {
	db := util.GetDB(ctx, s.db)

	updates := map[string]any{
		"update_time": p.Now,
		"lease_owner": "",
		"lease_until": int64(0),
	}

	if p.NextFireState == nil {
		updates["fire_id"] = nil
		updates["fire_state"] = nil
		updates["attempt_no"] = 0
		updates["next_attempt_at"] = int64(0)
	} else {
		updates["fire_state"] = *p.NextFireState
		updates["attempt_no"] = p.AttemptNo
		updates["next_attempt_at"] = p.NextAttemptAt
	}

	if p.NextFireAt > 0 {
		updates["next_fire_at"] = p.NextFireAt
	}
	if p.NewState != "" {
		updates["state"] = p.NewState
	}

	if p.Success {
		updates["last_success_at"] = p.Now
		updates["consecutive_failures"] = 0
	} else {
		updates["consecutive_failures"] = gorm.Expr("consecutive_failures + 1")
		updates["total_failures"] = gorm.Expr("total_failures + 1")
		if p.LastError != "" {
			updates["last_error"] = truncate(p.LastError, 1024)
		}
	}

	// 本次投递耗时过长导致落后时，被跳过的 occurrence 数量在这里一次性结算。
	if p.AddSkipped > 0 {
		updates["skipped_count"] = gorm.Expr("skipped_count + ?", p.AddSkipped)
	}

	res := db.Model(&schema.Schedule{}).
		Where("id = ? AND fire_id = ?", id, fireId).
		Updates(updates)
	if res.Error != nil {
		return false, sql.WrapError(res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (s *ScheduleStoreImpl) UpdateDefinition(
	ctx context.Context,
	sched *schema.Schedule,
) (bool, error) {
	db := util.GetDB(ctx, s.db)
	res := db.Model(&schema.Schedule{}).
		Where("id = ?", sched.Id).
		Updates(map[string]any{
			"name":          sched.Name,
			"schedule_type": sched.ScheduleType,
			"cron_expr":     sched.CronExpr,
			"timezone":      sched.Timezone,
			"fire_at":       sched.FireAt,
			"callback_url":  sched.CallbackUrl,
			"content_type":  sched.ContentType,
			"payload":       sched.Payload,
			"next_fire_at":  sched.NextFireAt,
			"update_time":   sched.UpdateTime,
		})
	if res.Error != nil {
		return false, sql.WrapError(res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (s *ScheduleStoreImpl) SetState(
	ctx context.Context,
	id uuid.UUID,
	from, to string,
	nextFireAt, now int64,
) (bool, error) {
	db := util.GetDB(ctx, s.db)

	updates := map[string]any{
		"state":       to,
		"update_time": now,
	}
	if nextFireAt > 0 {
		updates["next_fire_at"] = nextFireAt
	}

	res := db.Model(&schema.Schedule{}).
		Where("id = ? AND state = ?", id, from).
		Updates(updates)
	if res.Error != nil {
		return false, sql.WrapError(res.Error)
	}
	return res.RowsAffected > 0, nil
}

// DeleteCascade 硬删 schedule 并级联清理失败记录与死信，三者在同一事务内完成。
//
// 删定义、删证据，但审计日志会保留一份快照——正因为这里的死信与失败记录被一并抹掉，
// 快照才是事后唯一能回答「刚才删了什么」的东西。
func (s *ScheduleStoreImpl) DeleteCascade(ctx context.Context, id uuid.UUID) error {
	db := util.GetDB(ctx, s.db)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&schema.ScheduleAttemptFailure{}, "schedule_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&schema.ScheduleDeadLetter{}, "schedule_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&schema.Schedule{}, "id = ?", id).Error; err != nil {
			return err
		}
		return nil
	})
}

// truncate 按「字符」（而非字节）截断，避免切断 UTF-8 造成入库失败。
func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}
