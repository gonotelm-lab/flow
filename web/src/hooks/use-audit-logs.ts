import { useQuery } from "@tanstack/react-query";
import {
  listAdminAuditLogs,
  type AuditLogFilters,
} from "@/api/schedules";

/**
 * 审计日志只读，且不会自动刷新：它是历史记录，写入由操作触发。
 * 页面上的操作会在各自的 mutation 里 invalidate 这个 key。
 */
export function useAuditLogs(filters: AuditLogFilters) {
  return useQuery({
    queryKey: [
      "audit-logs",
      filters.page ?? 1,
      filters.pageSize ?? 20,
      filters.targetType ?? "",
      filters.targetId ?? "",
      filters.action ?? "",
      filters.namespace ?? "",
    ],
    queryFn: ({ signal }) => listAdminAuditLogs(filters, signal),
  });
}
