import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createSchedule,
  deleteSchedule,
  getSchedule,
  listScheduleDeadLetters,
  listSchedules,
  replayScheduleDeadLetter,
  stopSchedule,
  triggerSchedule,
  type DeadLetterFilters,
  type ScheduleFilters,
} from "@/api/schedules";
import type { Schedule } from "@/api/types";

function scheduleQueryKey(filters: ScheduleFilters) {
  return [
    "schedules",
    filters.page ?? 1,
    filters.pageSize ?? 20,
    filters.namespace ?? "",
    filters.scheduleType ?? "",
    filters.state ?? "",
    filters.id ?? "",
  ] as const;
}

/**
 * 列表每 5 秒刷新一次，与任务列表保持一致。
 *
 * 定时任务的状态变化是「后台发生」的（到期触发、失败重试、死信推进），运维面板如果
 * 不主动刷新，用户看到的就是一份过期快照。
 */
export function useSchedules(filters: ScheduleFilters) {
  return useQuery({
    queryKey: scheduleQueryKey(filters),
    queryFn: ({ signal }) => listSchedules(filters, signal),
    enabled: !!filters.namespace,
    refetchInterval: 5000,
  });
}

export function useSchedule(id: string | null) {
  return useQuery({
    queryKey: ["schedule", id],
    queryFn: ({ signal }) => getSchedule(id!, signal),
    enabled: !!id,
    refetchInterval: 5000,
  });
}

export function useScheduleDeadLetters(filters: DeadLetterFilters) {
  return useQuery({
    queryKey: [
      "schedule-dead-letters",
      filters.page ?? 1,
      filters.pageSize ?? 20,
      filters.namespace ?? "",
      filters.scheduleId ?? "",
      filters.state ?? "",
    ],
    queryFn: ({ signal }) => listScheduleDeadLetters(filters, signal),
    enabled: !!filters.namespace || !!filters.scheduleId,
    refetchInterval: 5000,
  });
}

function invalidateSchedules(qc: ReturnType<typeof useQueryClient>) {
  qc.invalidateQueries({ queryKey: ["schedules"] });
  qc.invalidateQueries({ queryKey: ["schedule"] });
  qc.invalidateQueries({ queryKey: ["schedule-dead-letters"] });
  // 审计日志会记录这些操作，顺带刷新
  qc.invalidateQueries({ queryKey: ["audit-logs"] });
}

export function useCreateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (schedule: Schedule) => createSchedule(schedule),
    onSuccess: () => invalidateSchedules(qc),
  });
}

export function useStopSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => stopSchedule(id),
    onSuccess: () => invalidateSchedules(qc),
  });
}

/** 立即投递一次；不推进 next_fire_at，不影响 cron 节奏。 */
export function useTriggerSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => triggerSchedule(id),
    onSuccess: () => invalidateSchedules(qc),
  });
}

export function useDeleteSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteSchedule(id),
    onSuccess: () => invalidateSchedules(qc),
  });
}

/** 重投会沿用原 Fire-Id，接收方据其去重。 */
export function useReplayDeadLetter() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => replayScheduleDeadLetter(id),
    onSuccess: () => invalidateSchedules(qc),
  });
}
