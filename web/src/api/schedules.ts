import { apiFetch, buildQuery, pageQueryParams } from "./client";
import type {
  ListAdminAuditLogsResponse,
  ListScheduleDeadLettersResponse,
  ListSchedulesResponse,
  Schedule,
} from "./types";

export type ScheduleFilters = {
  page?: number;
  pageSize?: number;
  namespace?: string;
  /** proto 枚举名，如 SCHEDULE_TYPE_CRON */
  scheduleType?: string;
  /** proto 枚举名，如 SCHEDULE_STATE_ACTIVE */
  state?: string;
  id?: string;
};

export function listSchedules(
  filters: ScheduleFilters = {},
  signal?: AbortSignal,
) {
  const { page = 1, pageSize = 20, namespace, scheduleType, state, id } = filters;
  return apiFetch<ListSchedulesResponse>(
    `/api/admin/v1/schedules${buildQuery({
      ...pageQueryParams(page, pageSize),
      namespace,
      schedule_type: scheduleType,
      state,
      id,
    })}`,
    { signal },
  );
}

export function getSchedule(id: string, signal?: AbortSignal) {
  return apiFetch<Schedule>(`/api/admin/v1/schedules/${id}`, { signal });
}

export function createSchedule(schedule: Schedule) {
  return apiFetch<Schedule>(`/api/admin/v1/schedules`, {
    method: "POST",
    body: JSON.stringify({ schedule }),
  });
}

export function updateSchedule(id: string, schedule: Schedule) {
  return apiFetch<Schedule>(`/api/admin/v1/schedules/${id}`, {
    method: "PUT",
    body: JSON.stringify({ schedule }),
  });
}

export function pauseSchedule(id: string) {
  return apiFetch<Schedule>(`/api/admin/v1/schedules/${id}/pause`, {
    method: "POST",
    body: "{}",
  });
}

export function resumeSchedule(id: string) {
  return apiFetch<Schedule>(`/api/admin/v1/schedules/${id}/resume`, {
    method: "POST",
    body: "{}",
  });
}

/**
 * 立即投递一次。仅运维面提供；不影响 cron 的自然节奏，也不会推进 next_fire_at。
 */
export function triggerSchedule(id: string) {
  return apiFetch<Schedule>(`/api/admin/v1/schedules/${id}/trigger`, {
    method: "POST",
    body: "{}",
  });
}

/**
 * 删除是硬删：定义、失败记录、死信一并消失。
 * 服务端会先取快照写进审计日志，因此事后仍能从审计里查到删了什么。
 */
export function deleteSchedule(id: string) {
  return apiFetch<void>(`/api/admin/v1/schedules/${id}`, { method: "DELETE" });
}

export type DeadLetterFilters = {
  page?: number;
  pageSize?: number;
  namespace?: string;
  scheduleId?: string;
  /** proto 枚举名，如 DEAD_LETTER_STATE_EXHAUSTED */
  state?: string;
};

export function listScheduleDeadLetters(
  filters: DeadLetterFilters = {},
  signal?: AbortSignal,
) {
  const { page = 1, pageSize = 20, namespace, scheduleId, state } = filters;
  return apiFetch<ListScheduleDeadLettersResponse>(
    `/api/admin/v1/schedule-dead-letters${buildQuery({
      ...pageQueryParams(page, pageSize),
      namespace,
      schedule_id: scheduleId,
      state,
    })}`,
    { signal },
  );
}

/**
 * 人工重投。服务端会沿用原 Fire-Id —— 接收方据此判断这是同一次事件的补投而非新事件。
 */
export function replayScheduleDeadLetter(id: string) {
  return apiFetch<void>(`/api/admin/v1/schedule-dead-letters/${id}/replay`, {
    method: "POST",
    body: "{}",
  });
}

/** 人工放弃。不给这个操作的话死信表会永久堆积。 */
export function discardScheduleDeadLetter(id: string) {
  return apiFetch<void>(`/api/admin/v1/schedule-dead-letters/${id}/discard`, {
    method: "POST",
    body: "{}",
  });
}

export type AuditLogFilters = {
  page?: number;
  pageSize?: number;
  targetType?: string;
  targetId?: string;
  action?: string;
  namespace?: string;
};

export function listAdminAuditLogs(
  filters: AuditLogFilters = {},
  signal?: AbortSignal,
) {
  const {
    page = 1,
    pageSize = 20,
    targetType,
    targetId,
    action,
    namespace,
  } = filters;
  return apiFetch<ListAdminAuditLogsResponse>(
    `/api/admin/v1/audit-logs${buildQuery({
      ...pageQueryParams(page, pageSize),
      target_type: targetType,
      target_id: targetId,
      action,
      namespace,
    })}`,
    { signal },
  );
}
