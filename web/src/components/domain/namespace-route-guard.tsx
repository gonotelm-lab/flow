import { useEffect } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { toast } from "sonner";
import { useNamespace } from "@/lib/namespace-context";

// 定时任务按命名空间隔离，所以需要先选命名空间；审计日志是全局视图，不需要。
const PROTECTED_PREFIXES = ["/tasks", "/schedules", "/workers"];

export function NamespaceRouteGuard({
  children,
}: {
  children: React.ReactNode;
}) {
  const { namespace } = useNamespace();
  const { pathname } = useLocation();

  const needsNamespace = PROTECTED_PREFIXES.some((p) =>
    pathname.startsWith(p),
  );

  useEffect(() => {
    if (needsNamespace && !namespace) {
      toast.info("请先选择命名空间");
    }
  }, [needsNamespace, namespace, pathname]);

  if (needsNamespace && !namespace) {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}
