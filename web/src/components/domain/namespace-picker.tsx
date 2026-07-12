import { useNavigate } from "react-router-dom";
import { ArrowRight, Boxes } from "lucide-react";
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
          <Skeleton key={i} className="h-28 rounded-lg" />
        ))}
      </div>
    );
  }

  if (isError) {
    return (
      <div className="rounded-lg border border-destructive/30 bg-destructive/8 p-4 text-sm text-destructive">
        无法加载命名空间列表。
        <button
          type="button"
          className="ml-2 cursor-pointer underline transition-colors duration-150 hover:text-destructive/80"
          onClick={() => refetch()}
        >
          重试
        </button>
      </div>
    );
  }

  if (data?.namespaces.length === 0) {
    return (
      <div className="flex flex-col items-center rounded-lg border border-dashed border-border py-20 text-center">
        <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
          <Boxes className="h-5 w-5 text-muted-foreground" />
        </div>
        <p className="font-medium">暂无命名空间</p>
        <p className="mt-1.5 text-sm text-muted-foreground">
          请先在服务端或通过命名空间页面创建
        </p>
      </div>
    );
  }

  if (variant === "table") {
    return (
      <div className="space-y-1.5">
        {data!.namespaces.map((ns) => (
          <button
            key={ns.name}
            type="button"
            className="flex w-full cursor-pointer items-center justify-between rounded-md border border-border px-4 py-3 text-left transition-colors duration-150 hover:bg-accent"
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
              "group cursor-pointer rounded-lg border border-border bg-card p-5 text-left shadow-sm transition-all duration-150 hover:border-foreground/20 hover:shadow-md",
              isCurrent && "border-foreground/30 ring-1 ring-foreground/10",
            )}
          >
            <div className="flex items-start justify-between gap-2">
              <p className="font-semibold tracking-tight">{ns.name}</p>
              <ArrowRight className="h-4 w-4 shrink-0 text-muted-foreground opacity-0 transition-all duration-150 group-hover:translate-x-0.5 group-hover:opacity-100" />
            </div>
            <p className="mt-2 line-clamp-2 text-sm text-muted-foreground">
              {ns.description || "无描述"}
            </p>
            {isCurrent && (
              <p className="mt-3 text-xs font-medium text-foreground/70">
                当前选中
              </p>
            )}
          </button>
        );
      })}
    </div>
  );
}
