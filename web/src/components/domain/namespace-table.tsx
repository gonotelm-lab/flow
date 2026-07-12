import { Fragment, useState } from "react";
import { toast } from "sonner";
import { Boxes, ChevronRight, Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
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
import { DEFAULT_PAGE_SIZE } from "@/lib/constants";
import { formatRelativeTime } from "@/lib/format";
import {
  useCreateNamespace,
  useNamespaces,
  useUpdateNamespace,
} from "@/hooks/use-namespaces";
import type { Namespace } from "@/api/types";
import { cn } from "@/lib/utils";

export function NamespaceTable() {
  const [page, setPage] = useState(1);
  const [expandedName, setExpandedName] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [newApiKey, setNewApiKey] = useState<string | null>(null);

  const { data, isLoading, isError } = useNamespaces(page, DEFAULT_PAGE_SIZE);
  const createMutation = useCreateNamespace();
  const updateMutation = useUpdateNamespace();

  const [createForm, setCreateForm] = useState({
    name: "",
    description: "",
    creator: "",
  });

  const [editForm, setEditForm] = useState({
    description: "",
    creator: "",
  });

  const handleCreate = async () => {
    if (!createForm.name.trim()) {
      toast.error("名称为必填项");
      return;
    }
    try {
      const ns = await createMutation.mutateAsync({
        namespace: createForm,
      });
      if (ns.apiKey) setNewApiKey(ns.apiKey);
      toast.success("命名空间已创建");
      setShowCreate(false);
      setCreateForm({ name: "", description: "", creator: "" });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "创建失败");
    }
  };

  const openEdit = (ns: Namespace) => {
    setExpandedName((prev) => (prev === ns.name ? null : ns.name));
    setEditForm({
      description: ns.description ?? "",
      creator: ns.creator ?? "",
    });
  };

  const handleUpdate = async (name: string) => {
    try {
      await updateMutation.mutateAsync({ name, body: editForm });
      toast.success("命名空间已更新");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "更新失败");
    }
  };

  const copyKey = (key: string) => {
    navigator.clipboard.writeText(key);
    toast.success("API 密钥已复制");
  };

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button size="sm" onClick={() => setShowCreate((v) => !v)}>
          {showCreate ? "取消" : "创建命名空间"}
        </Button>
      </div>

      {newApiKey && (
        <div className="rounded-lg border border-warning/30 bg-warning/8 p-4 text-sm">
          <p className="font-medium">API 密钥（仅显示一次）</p>
          <div className="mt-2 flex items-center gap-2">
            <code className="flex-1 break-all text-sm">
              {newApiKey}
            </code>
            <Button
              variant="outline"
              size="icon"
              className="h-8 w-8 shrink-0"
              onClick={() => copyKey(newApiKey)}
            >
              <Copy className="h-3 w-3" />
            </Button>
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="mt-2"
            onClick={() => setNewApiKey(null)}
          >
            关闭
          </Button>
        </div>
      )}

      {showCreate && (
        <div className="page-panel p-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <Input
              placeholder="名称 *"
              value={createForm.name}
              onChange={(e) =>
                setCreateForm((f) => ({ ...f, name: e.target.value }))
              }
            />
            <Input
              placeholder="描述"
              value={createForm.description}
              onChange={(e) =>
                setCreateForm((f) => ({ ...f, description: e.target.value }))
              }
            />
            <Input
              placeholder="创建者"
              value={createForm.creator}
              onChange={(e) =>
                setCreateForm((f) => ({ ...f, creator: e.target.value }))
              }
            />
          </div>
          <Button className="mt-3" size="sm" onClick={handleCreate}>
            创建
          </Button>
        </div>
      )}

      {isError && (
        <p className="text-sm text-destructive">加载命名空间失败。</p>
      )}

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-8 px-2" aria-hidden />
            <TableHead>名称</TableHead>
            <TableHead>描述</TableHead>
            <TableHead>创建者</TableHead>
            <TableHead>API 密钥</TableHead>
            <TableHead>创建时间</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading &&
            Array.from({ length: 4 }).map((_, i) => (
              <TableRow key={i}>
                <TableCell colSpan={6}>
                  <Skeleton className="h-8 w-full" />
                </TableCell>
              </TableRow>
            ))}

          {!isLoading && data?.namespaces.length === 0 && (
            <TableRow>
              <TableCell colSpan={6}>
                <EmptyState
                  icon={Boxes}
                  title="暂无命名空间"
                  description="创建命名空间以组织任务。"
                  action={{
                    label: "创建命名空间",
                    onClick: () => setShowCreate(true),
                  }}
                />
              </TableCell>
            </TableRow>
          )}

          {data?.namespaces.map((ns) => {
            const isOpen = expandedName === ns.name;
            return (
              <Fragment key={ns.name}>
                <TableRow
                  key={ns.name}
                  className={cn("cursor-pointer", isOpen && "bg-muted/40")}
                  data-state={isOpen ? "selected" : undefined}
                  aria-expanded={isOpen}
                  onClick={() => openEdit(ns)}
                >
                  <TableCell className="w-8 px-2">
                    <ChevronRight
                      className={cn(
                        "h-4 w-4 text-muted-foreground transition-transform duration-150",
                        isOpen && "rotate-90",
                      )}
                    />
                  </TableCell>
                  <TableCell className="font-medium">{ns.name}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {ns.description || "—"}
                  </TableCell>
                  <TableCell>{ns.creator || "—"}</TableCell>
                  <TableCell className="text-sm">
                    {ns.apiKeyPreview || "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {ns.createTime
                      ? formatRelativeTime(ns.createTime)
                      : "—"}
                  </TableCell>
                </TableRow>
                {isOpen && (
                  <TableRow key={`${ns.name}-edit`}>
                    <TableCell colSpan={6} className="bg-muted/30 p-4">
                      <div className="grid gap-3 sm:grid-cols-2">
                        <Input
                          placeholder="描述"
                          value={editForm.description}
                          onClick={(e) => e.stopPropagation()}
                          onChange={(e) =>
                            setEditForm((f) => ({
                              ...f,
                              description: e.target.value,
                            }))
                          }
                        />
                        <Input
                          placeholder="创建者"
                          value={editForm.creator}
                          onClick={(e) => e.stopPropagation()}
                          onChange={(e) =>
                            setEditForm((f) => ({
                              ...f,
                              creator: e.target.value,
                            }))
                          }
                        />
                      </div>
                      <Button
                        className="mt-3"
                        size="sm"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleUpdate(ns.name);
                        }}
                      >
                        保存
                      </Button>
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
