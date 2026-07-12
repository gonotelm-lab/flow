import { useQuery } from "@tanstack/react-query";
import { getWorker, listWorkers, type WorkerFilters } from "@/api/workers";

function workerQueryKey(filters: WorkerFilters) {
  return [
    "workers",
    filters.page ?? 1,
    filters.pageSize ?? 20,
    filters.namespace ?? "",
    filters.taskType ?? "",
  ] as const;
}

export function useWorkers(filters: WorkerFilters) {
  return useQuery({
    queryKey: workerQueryKey(filters),
    queryFn: ({ signal }) => listWorkers(filters, signal),
    enabled: !!filters.namespace,
    refetchInterval: 30000,
  });
}

export function useWorker(id: number | null) {
  return useQuery({
    queryKey: ["worker", id],
    queryFn: () => getWorker(String(id!)),
    enabled: id != null,
  });
}
