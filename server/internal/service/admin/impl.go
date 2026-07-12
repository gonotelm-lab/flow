package admin

import (
	"context"
	"crypto/rand"
	"strings"
	"time"

	adminv1 "github.com/gonotelm-lab/flow/api/admin/v1"
	apischema "github.com/gonotelm-lab/flow/api/schema/v1"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	store "github.com/gonotelm-lab/flow/server/internal/repository/store"
	srverr "github.com/gonotelm-lab/flow/server/internal/service/errors"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) createNamespace(
	ctx context.Context,
	namespace *apischema.Namespace,
) (*apischema.Namespace, error) {
	now := time.Now()
	ns, err := s.store.Namespace.Create(ctx,
		&reposchema.Namespace{
			Name:        namespace.GetName(),
			Description: namespace.GetDescription(),
			ApiKey:      generateApiKey(),
			Creator:     namespace.GetCreator(),
			CreateTime:  now.UnixMilli(),
			UpdateTime:  now.UnixMilli(),
		})
	if err != nil {
		if errors.Is(err, pkgerr.DuplicatedResource) {
			return nil, srverr.NamespaceExists
		}

		return nil, errors.WithMessage(err, "failed to create namespace")
	}

	return toApiNamespace(ns), nil
}

func (s *Service) getNamespace(
	ctx context.Context,
	name string,
) (*apischema.Namespace, error) {
	ns, err := s.store.Namespace.Get(ctx, name)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.NamespaceNotFound
		}

		return nil, errors.WithMessage(err, "failed to get namespace")
	}

	apiNs := toApiNamespace(ns)
	apiNs.ApiKey = ""

	return apiNs, nil
}

func toApiNamespace(ns *reposchema.Namespace) *apischema.Namespace {
	if ns == nil {
		return nil
	}

	return &apischema.Namespace{
		Name:          ns.Name,
		Description:   ns.Description,
		Creator:       ns.Creator,
		CreateTime:    timestamppb.New(time.UnixMilli(ns.CreateTime)),
		UpdateTime:    timestamppb.New(time.UnixMilli(ns.UpdateTime)),
		ApiKey:        ns.ApiKey,
		ApiKeyPreview: maskApiKey(ns.ApiKey),
	}
}

func normalizePage(pbPage *adminv1.PageRequest) (int32, int32) {
	page := int32(1)
	pageSize := int32(20)
	if pbPage != nil {
		if pbPage.GetPage() > 0 {
			page = pbPage.GetPage()
		}
		if pbPage.GetPageSize() > 0 {
			pageSize = pbPage.GetPageSize()
		}
	}
	return page, pageSize
}

func (s *Service) listNamespaces(
	ctx context.Context,
	page, pageSize int32,
) ([]*apischema.Namespace, int64, error) {
	offset := int((page - 1) * pageSize)
	limit := int(pageSize)

	namespaces, total, err := s.store.Namespace.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, errors.WithMessage(err, "failed to list namespaces")
	}

	result := make([]*apischema.Namespace, 0, len(namespaces))
	for _, ns := range namespaces {
		result = append(result, toApiNamespace(ns))
	}

	return result, total, nil
}

func (s *Service) updateNamespace(
	ctx context.Context,
	name, description, creator string,
) (*apischema.Namespace, error) {
	ns, err := s.store.Namespace.Get(ctx, name)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.NamespaceNotFound
		}
		return nil, errors.WithMessage(err, "failed to get namespace")
	}

	ns.Description = description
	ns.Creator = creator

	if err := s.store.Namespace.Update(ctx, ns); err != nil {
		return nil, errors.WithMessage(err, "failed to update namespace")
	}

	updated, err := s.store.Namespace.Get(ctx, name)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get updated namespace")
	}

	return toApiNamespace(updated), nil
}

func maskApiKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}

	const (
		prefixVisible = 3
		suffixVisible = 4
	)
	if len(apiKey) <= prefixVisible+suffixVisible {
		return strings.Repeat("*", len(apiKey))
	}

	maskLen := len(apiKey) - prefixVisible - suffixVisible
	return apiKey[:prefixVisible] + strings.Repeat("*", maskLen) + apiKey[len(apiKey)-suffixVisible:]
}

func generateApiKey() string {
	// TODO 加密后存储
	randText := rand.Text()
	return "sk-" + randText
}

func (s *Service) listTasks(
	ctx context.Context,
	page, pageSize int32,
	namespace, taskType, state string,
) ([]*apischema.Task, int64, error) {
	offset := int((page - 1) * pageSize)
	limit := int(pageSize)

	tasks, total, err := s.store.Task.List(ctx, &store.TaskListParams{
		Namespace: namespace,
		TaskType:  taskType,
		State:     state,
		Offset:    offset,
		Limit:     limit,
	})
	if err != nil {
		return nil, 0, errors.WithMessage(err, "failed to list tasks")
	}

	result := make([]*apischema.Task, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, toProtoTask(t))
	}

	return result, total, nil
}

func (s *Service) getTask(
	ctx context.Context,
	id string,
) (*apischema.Task, error) {
	taskId, err := uuid.Parse(id)
	if err != nil {
		return nil, pkgerr.InvalidArgument.WithDetail("task_id is invalid")
	}

	task, err := s.store.Task.Get(ctx, taskId)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.TaskNotFound
		}
		return nil, errors.WithMessage(err, "failed to get task")
	}

	return toProtoTask(task), nil
}

func (s *Service) cancelTask(
	ctx context.Context,
	id string,
) error {
	taskId, err := uuid.Parse(id)
	if err != nil {
		return pkgerr.InvalidArgument.WithDetail("task_id is invalid")
	}

	task, err := s.store.Task.Get(ctx, taskId)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return srverr.TaskNotFound
		}
		return errors.WithMessage(err, "failed to get task")
	}

	if task.State != apischema.TaskState_INITED.String() &&
		task.State != apischema.TaskState_RUNNING.String() {
		return srverr.TaskAlreadyEnded.WithDetail("task state: " + task.State)
	}

	nowMilli := time.Now().UnixMilli()
	task.State = apischema.TaskState_CANCELLED.String()
	task.UpdateTime = nowMilli

	ok, err := s.store.Task.Update(ctx, task)
	if err != nil {
		return errors.WithMessage(err, "failed to cancel task")
	}
	if !ok {
		return srverr.TaskAlreadyEnded
	}

	return nil
}

func (s *Service) deleteTask(
	ctx context.Context,
	id string,
) error {
	taskId, err := uuid.Parse(id)
	if err != nil {
		return pkgerr.InvalidArgument.WithDetail("task_id is invalid")
	}

	if err := s.store.Task.Delete(ctx, taskId); err != nil {
		return errors.WithMessage(err, "failed to delete task")
	}

	return nil
}

func (s *Service) listTaskEvents(
	ctx context.Context,
	taskID string,
	page, pageSize int32,
) ([]*apischema.TaskEvent, int64, error) {
	tId, err := uuid.Parse(taskID)
	if err != nil {
		return nil, 0, pkgerr.InvalidArgument.WithDetail("task_id is invalid")
	}

	offset := int((page - 1) * pageSize)
	limit := int(pageSize)

	events, total, err := s.store.TaskEvent.ListByTaskID(ctx, tId, offset, limit)
	if err != nil {
		return nil, 0, errors.WithMessage(err, "failed to list task events")
	}

	result := make([]*apischema.TaskEvent, 0, len(events))
	for _, e := range events {
		result = append(result, &apischema.TaskEvent{
			Id:         e.Id,
			TaskId:     e.TaskId.String(),
			EventType:  e.EventType,
			CreateTime: e.CreateTime,
			Payload:    e.Payload,
		})
	}

	return result, total, nil
}

func toProtoTask(task *reposchema.Task) *apischema.Task {
	if task == nil {
		return nil
	}

	state := apischema.TaskState_TASK_STATE_UNSPECIFIED
	if rawState, ok := apischema.TaskState_value[task.State]; ok {
		state = apischema.TaskState(rawState)
	}

	return &apischema.Task{
		Id:          task.Id.String(),
		Namespace:   task.Namespace,
		TaskType:    task.TaskType,
		Payload:     task.Payload,
		Result:      task.Result,
		Error:       task.Error,
		State:       state,
		CreateTime:  timestamppb.New(time.UnixMilli(task.CreateTime)),
		UpdateTime:  timestamppb.New(time.UnixMilli(task.UpdateTime)),
		NextRunTime: task.NextRunTime,
		MaxRetry:    int64(task.MaxRetry),
		AttemptNo:   int32(task.AttemptNo),
		WorkerId:    task.WorkerId,
	}
}
