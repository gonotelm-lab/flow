import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  cancelTask,
  deleteTask,
  getTask,
  listTaskEvents,
  listTasks,
  type TaskFilters,
} from "@/api/tasks";

function taskQueryKey(filters: TaskFilters) {
  return [
    "tasks",
    filters.page ?? 1,
    filters.pageSize ?? 20,
    filters.namespace ?? "",
    filters.taskType ?? "",
    filters.taskId ?? "",
    filters.state ?? "",
  ] as const;
}

export function useTasks(filters: TaskFilters) {
  return useQuery({
    queryKey: taskQueryKey(filters),
    queryFn: ({ signal }) => listTasks(filters, signal),
    enabled: !!filters.namespace,
    refetchInterval: 5000,
  });
}

export function useTask(id: string | null) {
  return useQuery({
    queryKey: ["task", id],
    queryFn: () => getTask(id!),
    enabled: !!id,
    refetchInterval: 5000,
  });
}

export function useTaskEvents(taskId: string | null, page = 1) {
  return useQuery({
    queryKey: ["task-events", taskId, page],
    queryFn: () => listTaskEvents(taskId!, page),
    enabled: !!taskId,
  });
}

export function useCancelTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: cancelTask,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tasks"] });
      qc.invalidateQueries({ queryKey: ["task"] });
    },
  });
}

export function useDeleteTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteTask,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tasks"] }),
  });
}
