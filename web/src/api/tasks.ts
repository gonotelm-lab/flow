import { apiFetch, buildQuery, pageQueryParams } from "./client";
import type { ListTaskEventsResponse, ListTasksResponse, Task } from "./types";

export type TaskFilters = {
  page?: number;
  pageSize?: number;
  namespace?: string;
  taskType?: string;
  taskId?: string;
  state?: string;
};

export function listTasks(filters: TaskFilters = {}, signal?: AbortSignal) {
  const { page = 1, pageSize = 20, namespace, taskType, taskId, state } = filters;
  return apiFetch<ListTasksResponse>(
    `/api/admin/v1/tasks${buildQuery({
      ...pageQueryParams(page, pageSize),
      namespace,
      task_type: taskType,
      id: taskId,
      state,
    })}`,
    { signal },
  );
}

export function getTask(id: string) {
  return apiFetch<Task>(`/api/admin/v1/tasks/${id}`);
}

export function cancelTask(id: string) {
  return apiFetch<void>(`/api/admin/v1/tasks/${id}/cancel`, {
    method: "POST",
    body: "{}",
  });
}

export function deleteTask(id: string) {
  return apiFetch<void>(`/api/admin/v1/tasks/${id}`, { method: "DELETE" });
}

export function listTaskEvents(taskId: string, page = 1, pageSize = 20) {
  return apiFetch<ListTaskEventsResponse>(
    `/api/admin/v1/tasks/${taskId}/events${buildQuery(pageQueryParams(page, pageSize))}`,
  );
}
