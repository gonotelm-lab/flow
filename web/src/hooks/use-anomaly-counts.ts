import { useQuery } from "@tanstack/react-query";
import { listTasks } from "@/api/tasks";
import { listWorkers } from "@/api/workers";
import { isHeartbeatStale } from "@/lib/format";

function useTaskStateCount(namespace: string | null, state: string) {
  return useQuery({
    queryKey: ["task-count", namespace, state],
    queryFn: ({ signal }) =>
      listTasks(
        { namespace: namespace!, page: 1, pageSize: 1, state },
        signal,
      ),
    enabled: !!namespace,
    refetchInterval: 5000,
    select: (data) => Number(data.page?.totalCount ?? 0),
  });
}

export function useTaskAnomalyCounts(namespace: string | null) {
  const failed = useTaskStateCount(namespace, "FAILED");
  const inited = useTaskStateCount(namespace, "INITED");

  return {
    failedCount: failed.data ?? 0,
    initedCount: inited.data ?? 0,
    isLoading: failed.isLoading || inited.isLoading,
  };
}

export function useWorkerStaleCount(namespace: string | null) {
  const query = useQuery({
    queryKey: ["worker-stale-count", namespace],
    queryFn: ({ signal }) =>
      listWorkers(
        { namespace: namespace!, page: 1, pageSize: 100 },
        signal,
      ),
    enabled: !!namespace,
    refetchInterval: 30000,
    select: (data) =>
      data.workers.filter(
        (w) => w.heartbeatTime && isHeartbeatStale(w.heartbeatTime),
      ).length,
  });

  return {
    staleCount: query.data ?? 0,
    isLoading: query.isLoading,
  };
}
