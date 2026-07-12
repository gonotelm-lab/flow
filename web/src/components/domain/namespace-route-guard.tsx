import { useEffect } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { toast } from "sonner";
import { useNamespace } from "@/lib/namespace-context";

const PROTECTED_PREFIXES = ["/tasks", "/workers"];

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
