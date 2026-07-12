import { apiFetch, buildQuery, pageQueryParams } from "./client";
import type { ListWorkersResponse, Worker } from "./types";

export type WorkerFilters = {
  page?: number;
  pageSize?: number;
  namespace?: string;
  taskType?: string;
};

export function listWorkers(filters: WorkerFilters = {}, signal?: AbortSignal) {
  const { page = 1, pageSize = 20, namespace, taskType } = filters;
  return apiFetch<ListWorkersResponse>(
    `/api/admin/v1/workers${buildQuery({
      ...pageQueryParams(page, pageSize),
      namespace,
      task_type: taskType,
    })}`,
    { signal },
  );
}

export function getWorker(id: string) {
  return apiFetch<Worker>(`/api/admin/v1/workers/${id}`);
}
