package admin

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	adminv1 "github.com/gonotelm-lab/flow/api/admin/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/audit"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	schedulesvc "github.com/gonotelm-lab/flow/server/internal/service/schedule"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// 定时任务的运维面。
//
// 与客户端面（api.schedule.v1.ScheduleService）共享同一套业务逻辑，额外提供：
// TriggerNow、死信处理、审计日志。所有写操作都会写审计——按设计，审计只覆盖
// admin 管理接口，客户端 gRPC 的操作不记。
// ---------------------------------------------------------------------------

func (s *Service) CreateSchedule(
	ctx context.Context,
	req *adminv1.CreateScheduleRequest,
) (*schemav1.Schedule, error) {
	if req == nil || req.GetSchedule() == nil {
		return nil, pkgerr.InvalidArgument.WithDetail("request schedule is empty")
	}

	created, err := s.scheduleCore.Create(ctx, schedulesvc.FromProtoSchedule(req.GetSchedule()))
	if err != nil {
		s.recordAudit(ctx, schema.AuditActionCreateSchedule, "", req.GetSchedule().GetNamespace(), nil, err)
		return nil, err
	}

	s.recordAudit(ctx, schema.AuditActionCreateSchedule, created.Id.String(), created.Namespace,
		scheduleSnapshot(created), nil)
	return schedulesvc.ToProtoSchedule(created), nil
}

func (s *Service) GetSchedule(
	ctx context.Context,
	req *adminv1.GetScheduleRequest,
) (*schemav1.Schedule, error) {
	id, err := parseScheduleID(req.GetId())
	if err != nil {
		return nil, err
	}

	found, err := s.scheduleCore.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return schedulesvc.ToProtoSchedule(found), nil
}

func (s *Service) ListSchedules(
	ctx context.Context,
	req *adminv1.ListSchedulesRequest,
) (*adminv1.ListSchedulesResponse, error) {
	if req == nil {
		return nil, pkgerr.InvalidArgument
	}

	page, pageSize := adminPaging(req.GetPage())

	scheduleType, err := schedulesvc.ScheduleTypeFromProto(req.GetScheduleType())
	if err != nil {
		return nil, err
	}
	state, err := schedulesvc.ScheduleStateFromProto(req.GetState())
	if err != nil {
		return nil, err
	}

	list, total, err := s.scheduleCore.List(ctx, schedulesvc.ListParams{
		Namespace:    req.GetNamespace(),
		ScheduleType: scheduleType,
		State:        state,
		Id:           req.GetId(),
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		return nil, err
	}

	out := &adminv1.ListSchedulesResponse{
		Page: &adminv1.PageResponse{
			Page:       int32(page),
			PageSize:   int32(pageSize),
			TotalCount: total,
		},
		Schedules: make([]*schemav1.Schedule, 0, len(list)),
	}
	for _, item := range list {
		out.Schedules = append(out.Schedules, schedulesvc.ToProtoSchedule(item))
	}

	return out, nil
}

// StopSchedule 单向停止一条 schedule，并在停止前取一份快照写进审计。
//
// 快照必须在停止之前取：stop 会清掉幂等键与 fire 槽位，事后光看这条 schedule
// 已经看不出「停之前它挂在哪个幂等键上、当时有没有 fire 在飞」。
func (s *Service) StopSchedule(
	ctx context.Context,
	req *adminv1.StopScheduleRequest,
) (*schemav1.Schedule, error) {
	id, err := parseScheduleID(req.GetId())
	if err != nil {
		return nil, err
	}

	var snapshot map[string]any
	var namespace string
	if existing, getErr := s.scheduleCore.Get(ctx, id); getErr == nil {
		snapshot = scheduleSnapshot(existing)
		namespace = existing.Namespace
		if existing.Busy() {
			snapshot["fire_in_flight"] = true
		}
	}

	stopped, err := s.scheduleCore.Stop(ctx, id)
	if err != nil {
		s.recordAudit(ctx, schema.AuditActionStopSchedule, req.GetId(), namespace, snapshot, err)
		return nil, err
	}

	s.recordAudit(ctx, schema.AuditActionStopSchedule, stopped.Id.String(), stopped.Namespace, snapshot, nil)
	return schedulesvc.ToProtoSchedule(stopped), nil
}

// DeleteSchedule 硬删定义并级联删除失败记录与死信。
//
// 快照必须在删除**之前**取：这些证据行随 schedule 一起消失，审计里的快照是事后唯一
// 能回答「刚才删了什么」的东西。
func (s *Service) DeleteSchedule(
	ctx context.Context,
	req *adminv1.DeleteScheduleRequest,
) (*emptypb.Empty, error) {
	id, err := parseScheduleID(req.GetId())
	if err != nil {
		return nil, err
	}

	var snapshot map[string]any
	var namespace string
	if existing, getErr := s.scheduleCore.Get(ctx, id); getErr == nil {
		snapshot = scheduleSnapshot(existing)
		namespace = existing.Namespace
		if existing.Busy() {
			snapshot["fire_in_flight"] = true
		}
	}

	if err := s.scheduleCore.Delete(ctx, id); err != nil {
		s.recordAudit(ctx, schema.AuditActionDeleteSchedule, req.GetId(), namespace, snapshot, err)
		return nil, err
	}

	s.recordAudit(ctx, schema.AuditActionDeleteSchedule, req.GetId(), namespace, snapshot, nil)
	return &emptypb.Empty{}, nil
}

// TriggerSchedule 立即投递一次，不推进 next_fire_at。
func (s *Service) TriggerSchedule(
	ctx context.Context,
	req *adminv1.TriggerScheduleRequest,
) (*schemav1.Schedule, error) {
	id, err := parseScheduleID(req.GetId())
	if err != nil {
		return nil, err
	}

	cur, err := s.scheduleCore.TriggerNow(ctx, id)
	if err != nil {
		s.recordAudit(ctx, schema.AuditActionTriggerSchedule, req.GetId(), "", nil, err)
		return nil, err
	}

	s.recordAudit(ctx, schema.AuditActionTriggerSchedule, cur.Id.String(), cur.Namespace, nil, nil)
	return schedulesvc.ToProtoSchedule(cur), nil
}

// ---- 死信 ----

func (s *Service) ListScheduleDeadLetters(
	ctx context.Context,
	req *adminv1.ListScheduleDeadLettersRequest,
) (*adminv1.ListScheduleDeadLettersResponse, error) {
	if req == nil {
		return nil, pkgerr.InvalidArgument
	}

	page, pageSize := adminPaging(req.GetPage())

	state, err := schedulesvc.DeadLetterStateFromProto(req.GetState())
	if err != nil {
		return nil, err
	}

	list, total, err := s.scheduleCore.ListDeadLetters(ctx, schedulesvc.DeadLetterListParams{
		Namespace:  req.GetNamespace(),
		ScheduleId: req.GetScheduleId(),
		State:      state,
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		return nil, err
	}

	out := &adminv1.ListScheduleDeadLettersResponse{
		Page: &adminv1.PageResponse{
			Page:       int32(page),
			PageSize:   int32(pageSize),
			TotalCount: total,
		},
		DeadLetters: make([]*adminv1.ScheduleDeadLetter, 0, len(list)),
	}
	for _, dl := range list {
		out.DeadLetters = append(out.DeadLetters, toProtoDeadLetter(dl))
	}

	return out, nil
}

func (s *Service) ReplayScheduleDeadLetter(
	ctx context.Context,
	req *adminv1.ReplayScheduleDeadLetterRequest,
) (*emptypb.Empty, error) {
	id, err := parseScheduleID(req.GetId())
	if err != nil {
		return nil, err
	}

	// 先取一次死信记录：审计要记它自己的 namespace，失败路径也要记得住。
	dl, _ := s.scheduleCore.GetDeadLetter(ctx, id)

	if err := s.scheduleCore.ReplayDeadLetter(ctx, id); err != nil {
		s.recordDeadLetterAudit(ctx, schema.AuditActionReplayDeadLetter, id, dl, err)
		return nil, err
	}

	s.recordDeadLetterAudit(ctx, schema.AuditActionReplayDeadLetter, id, dl, nil)
	return &emptypb.Empty{}, nil
}

// ---- 审计 ----

func (s *Service) ListAdminAuditLogs(
	ctx context.Context,
	req *adminv1.ListAdminAuditLogsRequest,
) (*adminv1.ListAdminAuditLogsResponse, error) {
	if req == nil {
		return nil, pkgerr.InvalidArgument
	}

	page, pageSize := adminPaging(req.GetPage())
	offset := (page - 1) * pageSize

	logs, total, err := s.store.AdminAuditLog.List(ctx, &store.AdminAuditLogListParams{
		TargetType: req.GetTargetType(),
		TargetId:   req.GetTargetId(),
		Action:     req.GetAction(),
		Namespace:  req.GetNamespace(),
		Offset:     offset,
		Limit:      pageSize,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to list audit logs")
	}

	out := &adminv1.ListAdminAuditLogsResponse{
		Page: &adminv1.PageResponse{
			Page:       int32(page),
			PageSize:   int32(pageSize),
			TotalCount: total,
		},
		Logs: make([]*adminv1.AdminAuditLog, 0, len(logs)),
	}
	for _, log := range logs {
		out.Logs = append(out.Logs, &adminv1.AdminAuditLog{
			Id:           log.Id.String(),
			Action:       log.Action,
			TargetType:   log.TargetType,
			TargetId:     log.TargetId,
			Namespace:    log.Namespace,
			Operator:     log.Operator,
			SourceIp:     log.SourceIp,
			UserAgent:    log.UserAgent,
			Source:       log.Source,
			Snapshot:     string(log.Snapshot),
			Result:       log.Result,
			ErrorMessage: log.ErrorMessage,
			CreateTime:   tsFromMillis(log.CreateTime),
		})
	}

	return out, nil
}

// recordAudit 投递一条审计条目。
//
// 审计是异步写的（见 internal/audit），因此这里不会拖慢业务。带失败时也照记一条
// ERROR，运维才能看出「谁试图删但没删掉」。
func (s *Service) recordAudit(
	ctx context.Context,
	action, targetId, namespace string,
	snapshot map[string]any,
	callErr error,
) {
	if s.audit == nil {
		return
	}

	entry := audit.Entry{
		Action:     action,
		TargetType: schema.AuditTargetSchedule,
		TargetId:   targetId,
		Namespace:  namespace,
		Source:     schema.AuditSourceAdminHTTP,
		Result:     schema.AuditResultOk,
	}
	// Operator 按设计留空：当前没有用户体系，服务端无法验证身份。
	entry.SourceIp, entry.UserAgent = clientInfo(ctx)

	if snapshot != nil {
		if raw, err := json.Marshal(snapshot); err == nil {
			entry.Snapshot = raw
		}
	}
	if callErr != nil {
		entry.Result = schema.AuditResultError
		entry.ErrorMessage = callErr.Error()
	}

	s.audit.Record(entry)
}

// recordDeadLetterAudit 记录死信操作。
//
// 与 recordAudit 的区别是 target_type 记 SCHEDULE_DEAD_LETTER、namespace 取死信自身的：
// 操作对象是死信记录而不是 schedule，混用会让审计里「这条到底动了什么」变得含糊。
func (s *Service) recordDeadLetterAudit(
	ctx context.Context,
	action string,
	id uuid.UUID,
	dl *schema.ScheduleDeadLetter,
	callErr error,
) {
	if s.audit == nil {
		return
	}

	entry := audit.Entry{
		Action:     action,
		TargetType: schema.AuditTargetScheduleDeadLetter,
		TargetId:   id.String(),
		Source:     schema.AuditSourceAdminHTTP,
		Result:     schema.AuditResultOk,
	}
	entry.SourceIp, entry.UserAgent = clientInfo(ctx)

	if dl != nil {
		entry.Namespace = dl.Namespace
		// 死信上的关键信息一并留档：它可能随后被重投或被 stop 改掉状态。
		if raw, err := json.Marshal(map[string]any{
			"dead_letter_id":   dl.Id.String(),
			"schedule_id":      dl.ScheduleId.String(),
			"fire_id":          dl.FireId.String(),
			"scheduled_time":   dl.ScheduledTime,
			"attempt_count":    dl.AttemptCount,
			"state_before":     dl.State,
			"last_http_status": dl.LastHttpStatus,
			"last_error":       dl.LastError,
		}); err == nil {
			entry.Snapshot = raw
		}
	}

	if callErr != nil {
		entry.Result = schema.AuditResultError
		entry.ErrorMessage = callErr.Error()
	}

	s.audit.Record(entry)
}

// scheduleSnapshot 抽取审计快照要保留的定义字段。
//
// 之所以要快照：Delete 是硬删加级联，死信与失败记录一起没了，快照是事后唯一能回答
// 「刚才删了什么、当时失败成什么样」的东西。
func scheduleSnapshot(s *schema.Schedule) map[string]any {
	if s == nil {
		return nil
	}

	return map[string]any{
		"id":                   s.Id.String(),
		"namespace":            s.Namespace,
		"name":                 s.Name,
		"schedule_type":        s.ScheduleType,
		"cron_expr":            s.CronExpr,
		"timezone":             s.Timezone,
		"fire_at":              s.FireAt,
		"callback_url":         s.CallbackUrl,
		"content_type":         s.ContentType,
		"idempotency_key":      schedulesvc.IdempotencyKeyOf(s),
		"state_before":         s.State,
		"consecutive_failures": s.ConsecutiveFailures,
		"total_failures":       s.TotalFailures,
		"skipped_count":        s.SkippedCount,
		"last_error":           s.LastError,
	}
}

// clientInfo 尽力取出调用方信息。
//
// 注意：当前服务端没有任何鉴权，所以这些只是「拿得到就拿」的痕迹，不构成安全审计。
// x-forwarded-for / x-real-ip 能取到是因为 adminserver 给 grpc-gateway 配了透传。
func clientInfo(ctx context.Context) (ip, userAgent string) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ""
	}

	ip = firstMeta(md, "x-forwarded-for")
	if ip != "" {
		// 可能是 "client, proxy1, proxy2"，取第一个
		ip = strings.TrimSpace(strings.Split(ip, ",")[0])
	} else {
		ip = firstMeta(md, "x-real-ip")
	}

	userAgent = firstMeta(md, "grpcgateway-user-agent")
	if userAgent == "" {
		userAgent = firstMeta(md, "user-agent")
	}

	return ip, userAgent
}

func firstMeta(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func toProtoDeadLetter(dl *schema.ScheduleDeadLetter) *adminv1.ScheduleDeadLetter {
	if dl == nil {
		return nil
	}
	return &adminv1.ScheduleDeadLetter{
		Id:             dl.Id.String(),
		ScheduleId:     dl.ScheduleId.String(),
		Namespace:      dl.Namespace,
		FireId:         dl.FireId.String(),
		ScheduledTime:  dl.ScheduledTime,
		AttemptCount:   int32(dl.AttemptCount),
		State:          schedulesvc.DeadLetterStateToProto(dl.State),
		NextAttemptAt:  dl.NextAttemptAt,
		LastHttpStatus: int32(dl.LastHttpStatus),
		LastError:      dl.LastError,
		CreateTime:     tsFromMillis(dl.CreateTime),
		UpdateTime:     tsFromMillis(dl.UpdateTime),
	}
}

func parseScheduleID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, pkgerr.InvalidArgument.WithDetail("schedule id is invalid")
	}
	return id, nil
}

func adminPaging(page *adminv1.PageRequest) (int, int) {
	if page == nil {
		return 1, 20
	}

	p := int(page.GetPage())
	if p <= 0 {
		p = 1
	}

	size := int(page.GetPageSize())
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	return p, size
}

func tsFromMillis(millis int64) *timestamppb.Timestamp {
	if millis <= 0 {
		return nil
	}
	return timestamppb.New(time.UnixMilli(millis))
}
