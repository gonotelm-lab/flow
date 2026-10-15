import { useState } from "react";
import { Search, ShieldCheck } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { EmptyState } from "@/components/domain/empty-state";
import { Pagination } from "@/components/domain/pagination";
import { useAuditLogs } from "@/hooks/use-audit-logs";
import { DEFAULT_PAGE_SIZE } from "@/lib/constants";
import { formatTimestamp } from "@/lib/format";
import { useNamespace } from "@/lib/namespace-context";
import type { AdminAuditLog } from "@/api/types";

const ACTION_LABEL: Record<string, string> = {
  CREATE_SCHEDULE: "创建定时任务",
  UPDATE_SCHEDULE: "修改定时任务",
  PAUSE_SCHEDULE: "暂停定时任务",
  RESUME_SCHEDULE: "恢复定时任务",
  DELETE_SCHEDULE: "删除定时任务",
  TRIGGER_SCHEDULE: "手动触发",
  REPLAY_DEAD_LETTER: "重投死信",
  DISCARD_DEAD_LETTER: "放弃死信",
};

/**
 * 审计日志页。
 *
 * 只覆盖 admin 管理接口的操作：客户端走 gRPC 注册/修改/暂停不会留下记录。
 * operator 字段当前恒为空（没有用户体系），所以身份这一栏只能靠来源 IP 与 UA。
 */
export function AuditTable() {
  const { namespace } = useNamespace();

  const [page, setPage] = useState(1);
  const [actionFilter, setActionFilter] = useState("");
  const [actionDraft, setActionDraft] = useState("");
  const [snapshot, setSnapshot] = useState<AdminAuditLog | null>(null);

  const { data, isLoading } = useAuditLogs({
    page,
    pageSize: DEFAULT_PAGE_SIZE,
    namespace: namespace ?? undefined,
    action: actionFilter,
  });

  const rows = data?.logs ?? [];
  const totalCount = data?.page?.totalCount ?? 0;

  return (
    <div className="page-panel">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b p-3">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <ShieldCheck className="h-4 w-4" />
          仅记录管理接口的操作；operator 待用户体系接入后填充
        </div>

        <div className="flex items-center gap-3">
          <Input
            className="w-56"
            placeholder="操作名，如 DELETE_SCHEDULE"
            value={actionDraft}
            onChange={(e) => setActionDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                setActionFilter(actionDraft.trim());
                setPage(1);
              }
            }}
          />
          <Button
            variant="outline"
            size="icon"
            aria-label="查询"
            onClick={() => {
              setActionFilter(actionDraft.trim());
              setPage(1);
            }}
          >
            <Search className="h-4 w-4" />
          </Button>
        </div>
      </div>

      <Table>
        <colgroup>
          <col style={{ width: "12rem" }} />
          <col style={{ width: "9rem" }} />
          <col style={{ minWidth: "14rem" }} />
          <col style={{ width: "9rem" }} />
          <col style={{ width: "9rem" }} />
          <col style={{ width: "5rem" }} />
          <col style={{ width: "5rem" }} />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead>时间</TableHead>
            <TableHead>操作</TableHead>
            <TableHead>目标</TableHead>
            <TableHead>命名空间</TableHead>
            <TableHead>来源</TableHead>
            <TableHead>结果</TableHead>
            <TableHead>快照</TableHead>
          </TableRow>
        </TableHeader>

        <TableBody>
          {isLoading &&
            Array.from({ length: 6 }).map((_, i) => (
              <TableRow key={i}>
                <TableCell colSpan={7} className="p-3">
                  <Skeleton className="h-6 w-full" />
                </TableCell>
              </TableRow>
            ))}

          {!isLoading && rows.length === 0 && (
            <TableRow>
              <TableCell colSpan={7} className="p-0">
                <EmptyState
                  icon={ShieldCheck}
                  title="暂无审计记录"
                  description="在管理接口上创建、修改、删除定时任务后会出现在这里"
                />
              </TableCell>
            </TableRow>
          )}

          {!isLoading &&
            rows.map((log) => (
              <TableRow key={log.id}>
                <TableCell className="p-0">
                  <div className="flex h-11 items-center px-3 text-sm tabular-nums text-muted-foreground">
                    {formatTimestamp(log.createTime)}
                  </div>
                </TableCell>

                <TableCell className="p-0">
                  <div className="flex h-11 items-center px-3 text-sm">
                    {ACTION_LABEL[log.action] ?? log.action}
                  </div>
                </TableCell>

                <TableCell className="p-0">
                  <div className="flex h-11 items-center px-3 text-sm">
                    <span className="truncate" title={log.targetId}>
                      {log.targetType}
                      {log.targetId ? ` · ${log.targetId}` : ""}
                    </span>
                  </div>
                </TableCell>

                <TableCell className="p-0">
                  <div className="flex h-11 items-center px-3 text-sm text-muted-foreground">
                    {log.namespace || "—"}
                  </div>
                </TableCell>

                <TableCell className="p-0">
                  <div className="flex h-11 items-center px-3 text-xs text-muted-foreground">
                    <span className="truncate" title={log.userAgent}>
                      {log.sourceIp || "—"}
                    </span>
                  </div>
                </TableCell>

                <TableCell className="p-0">
                  <div className="flex h-11 items-center px-3">
                    <Badge variant={log.result === "OK" ? "success" : "destructive"}>
                      {log.result === "OK" ? "成功" : "失败"}
                    </Badge>
                  </div>
                </TableCell>

                <TableCell className="p-0">
                  <div className="flex h-11 items-center px-3">
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={!log.snapshot}
                      className={!log.snapshot ? "opacity-40" : undefined}
                      onClick={() => setSnapshot(log)}
                    >
                      查看
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
        </TableBody>
      </Table>

      <div className="p-3">
        <Pagination
          page={page}
          pageSize={DEFAULT_PAGE_SIZE}
          totalCount={totalCount}
          onPageChange={setPage}
        />
      </div>

      <Dialog open={!!snapshot} onOpenChange={(open) => !open && setSnapshot(null)}>
        <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
          <DialogHeader>
            <DialogTitle>操作快照</DialogTitle>
            <DialogDescription>
              删除是硬删且级联清理失败记录与死信，这份快照是事后唯一能回答「删了什么」的依据。
            </DialogDescription>
          </DialogHeader>

          {snapshot?.errorMessage && (
            <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
              {snapshot.errorMessage}
            </div>
          )}

          <pre className="max-h-[50vh] overflow-auto rounded-md border bg-muted/40 p-3 text-xs">
            {prettyJSON(snapshot?.snapshot)}
          </pre>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function prettyJSON(raw?: string): string {
  if (!raw) return "（无快照）";
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}
