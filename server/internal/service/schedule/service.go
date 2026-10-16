package schedule

import (
	"context"

	schedulev1 "github.com/gonotelm-lab/flow/api/schedule/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Service 是客户端面（api.schedule.v1.ScheduleService，仅 gRPC / 7091）。
//
// 这里不写审计日志——审计只覆盖 admin 管理接口的操作。TriggerNow、死信处理与审计
// 都挂在运维面（api.admin.v1.AdminService），不暴露给客户端。
type Service struct {
	schedulev1.UnimplementedScheduleServiceServer

	core *Core
}

func NewService(store *repository.Store) schedulev1.ScheduleServiceServer {
	return &Service{core: NewCore(store)}
}

func (s *Service) Register(
	ctx context.Context,
	req *schedulev1.RegisterRequest,
) (*schemav1.Schedule, error) {
	if req == nil || req.GetSchedule() == nil {
		return nil, pkgerr.InvalidArgument.WithDetail("request schedule is empty")
	}

	created, err := s.core.Create(ctx, FromProtoSchedule(req.GetSchedule()))
	if err != nil {
		return nil, err
	}
	return ToProtoSchedule(created), nil
}

func (s *Service) Get(
	ctx context.Context,
	req *schedulev1.GetRequest,
) (*schemav1.Schedule, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}

	found, err := s.core.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return ToProtoSchedule(found), nil
}

func (s *Service) List(
	ctx context.Context,
	req *schedulev1.ListRequest,
) (*schedulev1.ListResponse, error) {
	if req == nil {
		return nil, pkgerr.InvalidArgument
	}

	page, pageSize := requestPaging(req.GetPage())

	scheduleType, err := ScheduleTypeFromProto(req.GetScheduleType())
	if err != nil {
		return nil, err
	}
	state, err := ScheduleStateFromProto(req.GetState())
	if err != nil {
		return nil, err
	}

	list, total, err := s.core.List(ctx, ListParams{
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

	out := &schedulev1.ListResponse{
		Page: &schemav1.PageResponse{
			Page:       int32(page),
			PageSize:   int32(pageSize),
			TotalCount: total,
		},
		Schedules: make([]*schemav1.Schedule, 0, len(list)),
	}
	for _, item := range list {
		out.Schedules = append(out.Schedules, ToProtoSchedule(item))
	}

	return out, nil
}

func (s *Service) Stop(
	ctx context.Context,
	req *schedulev1.StopRequest,
) (*schemav1.Schedule, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}

	stopped, err := s.core.Stop(ctx, id)
	if err != nil {
		return nil, err
	}
	return ToProtoSchedule(stopped), nil
}

func (s *Service) Delete(
	ctx context.Context,
	req *schedulev1.DeleteRequest,
) (*emptypb.Empty, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}

	if err := s.core.Delete(ctx, id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, pkgerr.InvalidArgument.WithDetail("schedule id is invalid")
	}
	return id, nil
}

func requestPaging(page *schemav1.PageRequest) (int, int) {
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
