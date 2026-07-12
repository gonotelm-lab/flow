import { Fragment, useEffect, useState } from "react";
import { Cpu } from "lucide-react";
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
import { WorkerExpandPanel } from "@/components/domain/worker-expand-panel";
import {
  formatRelativeTime,
  isHeartbeatStale,
} from "@/lib/format";
import { DEFAULT_PAGE_SIZE } from "@/lib/constants";
import { useNamespace } from "@/lib/namespace-context";
import { useWorkers } from "@/hooks/use-workers";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { cn } from "@/lib/utils";

export function WorkerTable() {
  const { namespace: selectedNamespace } = useNamespace();
  const [page, setPage] = useState(1);
  const [taskType, setTaskType] = useState("");
  const debouncedTaskType = useDebouncedValue(taskType.trim());
  const [expandedId, setExpandedId] = useState<number | null>(null);

  useEffect(() => {
    setPage(1);
    setExpandedId(null);
  }, [selectedNamespace, debouncedTaskType]);

  const { data, isLoading, isError } = useWorkers({
    page,
    pageSize: DEFAULT_PAGE_SIZE,
    namespace: selectedNamespace ?? undefined,
    taskType: debouncedTaskType || undefined,
  });

  return (
    <div className="space-y-4">
      <div className="flex gap-3">
        <Input
          placeholder="任务类型（支持模糊匹配）"
          className="h-8 w-44"
          value={taskType}
          onChange={(e) => setTaskType(e.target.value)}
        />
      </div>

      {isError && (
        <p className="text-sm text-destructive">加载工作节点失败。</p>
      )}

      <Table>
        <colgroup>
          <col style={{ width: "2.5rem" }} />
          <col style={{ width: "42%" }} />
          <col style={{ width: "20%" }} />
          <col style={{ width: "14%" }} />
          <col style={{ width: "10%" }} />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10 px-2">ID</TableHead>
            <TableHead>名称</TableHead>
            <TableHead>类型</TableHead>
            <TableHead>心跳</TableHead>
            <TableHead>成功 / 总计</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading &&
            Array.from({ length: 5 }).map((_, i) => (
              <TableRow key={i}>
                <TableCell colSpan={5}>
                  <Skeleton className="h-8 w-full" />
                </TableCell>
              </TableRow>
            ))}

          {!isLoading && data?.workers.length === 0 && (
            <TableRow>
              <TableCell colSpan={5}>
                <EmptyState
                  icon={Cpu}
                  title="暂无工作节点"
                  description="工作节点连接队列后会自动注册。"
                />
              </TableCell>
            </TableRow>
          )}

          {data?.workers.map((worker) => {
            const isOpen = expandedId === worker.id;
            const stale =
              worker.heartbeatTime &&
              isHeartbeatStale(worker.heartbeatTime);

            return (
              <Fragment key={worker.id}>
                <TableRow
                  className="cursor-pointer"
                  data-state={isOpen ? "selected" : undefined}
                  onClick={() =>
                    setExpandedId((prev) =>
                      prev === worker.id ? null : worker.id,
                    )
                  }
                >
                  <TableCell className="px-2 font-mono text-xs tabular-nums">
                    {worker.id}
                  </TableCell>
                  <TableCell
                    className="max-w-0 truncate whitespace-nowrap"
                    title={worker.name || undefined}
                  >
                    {worker.name || "—"}
                  </TableCell>
                  <TableCell className="max-w-0 truncate text-muted-foreground">
                    {worker.taskType}
                  </TableCell>
                  <TableCell
                    className={cn(stale && "text-destructive")}
                  >
                    {formatRelativeTime(worker.heartbeatTime)}
                  </TableCell>
                  <TableCell>
                    {worker.successDealt ?? 0} / {worker.totalDealt ?? 0}
                  </TableCell>
                </TableRow>
                {isOpen && (
                  <TableRow key={`${worker.id}-detail`}>
                    <TableCell colSpan={5} className="bg-muted/40 p-0">
                      <WorkerExpandPanel workerId={worker.id} />
                    </TableCell>
                  </TableRow>
                )}
              </Fragment>
            );
          })}
        </TableBody>
      </Table>

      {data?.page && (
        <Pagination
          page={data.page.page}
          pageSize={data.page.pageSize}
          totalCount={Number(data.page.totalCount)}
          onPageChange={setPage}
        />
      )}
    </div>
  );
}
