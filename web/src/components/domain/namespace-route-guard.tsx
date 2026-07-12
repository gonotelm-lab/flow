import { Navigate, useLocation } from "react-router-dom";
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

  if (needsNamespace && !namespace) {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}
