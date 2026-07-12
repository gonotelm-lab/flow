import { useNavigate } from "react-router-dom";
import { Boxes } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { useNamespaces } from "@/hooks/use-namespaces";
import { useNamespace } from "@/lib/namespace-context";
import { cn } from "@/lib/utils";

type NamespacePickerProps = {
  onSelected?: () => void;
  variant?: "cards" | "table";
};

export function NamespacePicker({
  onSelected,
  variant = "cards",
}: NamespacePickerProps) {
  const { namespace: current, setNamespace } = useNamespace();
  const navigate = useNavigate();
  const { data, isLoading, isError, refetch } = useNamespaces(1, 100);

  const handleSelect = (name: string) => {
    setNamespace(name);
    onSelected?.();
    navigate("/tasks");
  };

  if (isLoading) {
    return (
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-24 rounded-lg" />
        ))}
      </div>
    );
  }

  if (isError) {
    return (
      <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
        无法加载命名空间列表。
        <button
          type="button"
          className="ml-2 underline"
          onClick={() => refetch()}
        >
          重试
        </button>
      </div>
    );
  }

  if (data?.namespaces.length === 0) {
    return (
      <div className="flex flex-col items-center rounded-lg border border-dashed border-border py-16 text-center">
        <Boxes className="mb-3 h-10 w-10 text-muted-foreground" />
        <p className="font-medium">暂无命名空间</p>
        <p className="mt-1 text-sm text-muted-foreground">
          请先在服务端或通过命名空间页面创建
        </p>
      </div>
    );
  }

  if (variant === "table") {
    return (
      <div className="space-y-2">
        {data!.namespaces.map((ns) => (
          <button
            key={ns.name}
            type="button"
            className="flex w-full cursor-pointer items-center justify-between rounded-md border border-border px-4 py-3 text-left hover:bg-muted/50"
            onClick={() => handleSelect(ns.name)}
          >
            <span className="font-medium">{ns.name}</span>
            <span className="text-sm text-muted-foreground">
              {ns.description || "—"}
            </span>
          </button>
        ))}
      </div>
    );
  }

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {data!.namespaces.map((ns) => {
        const isCurrent = current === ns.name;
        return (
          <button
            key={ns.name}
            type="button"
            onClick={() => handleSelect(ns.name)}
            className={cn(
              "cursor-pointer rounded-lg border border-border bg-card p-4 text-left transition-colors hover:border-primary/40 hover:bg-muted/30",
              isCurrent && "border-primary ring-1 ring-primary/20",
            )}
          >
            <p className="font-semibold">{ns.name}</p>
            <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">
              {ns.description || "无描述"}
            </p>
            {isCurrent && (
              <p className="mt-2 text-xs text-primary">当前选中</p>
            )}
          </button>
        );
      })}
    </div>
  );
}
