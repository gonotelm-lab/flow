import { useState } from "react";
import { Link, useLocation } from "react-router-dom";
import {
  Boxes,
  ChevronLeft,
  ChevronRight,
  Cpu,
  Home,
  ListTodo,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useNamespace } from "@/lib/namespace-context";
import {
  useTaskAnomalyCounts,
  useWorkerStaleCount,
} from "@/hooks/use-anomaly-counts";

const NAV_ITEMS = [
  { to: "/", icon: Home, label: "首页" },
  { to: "/tasks", icon: ListTodo, label: "任务" },
  { to: "/workers", icon: Cpu, label: "工作节点" },
  { to: "/namespaces", icon: Boxes, label: "命名空间" },
] as const;

const STORAGE_KEY = "flow-sidebar-collapsed";

export function Sidebar() {
  const { pathname } = useLocation();
  const { namespace } = useNamespace();
  const { failedCount, initedCount } = useTaskAnomalyCounts(namespace);
  const taskAnomalyCount = failedCount + initedCount;
  const { staleCount: workerStaleCount } = useWorkerStaleCount(namespace);
  const [collapsed, setCollapsed] = useState(() =>
    localStorage.getItem(STORAGE_KEY) === "1",
  );

  const toggle = () => {
    setCollapsed((c) => {
      const next = !c;
      localStorage.setItem(STORAGE_KEY, next ? "1" : "0");
      return next;
    });
  };

  return (
    <aside
      className={cn(
        "flex shrink-0 flex-col border-r border-sidebar-border bg-sidebar transition-[width] duration-200 ease-out",
        collapsed ? "w-14" : "w-52",
      )}
    >
      <div
        className={cn(
          "flex border-b border-sidebar-border",
          collapsed
            ? "flex-col items-center gap-1 py-2"
            : "h-12 items-center justify-between px-3",
        )}
      >
        <div className="flex min-w-0 items-center">
          <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-foreground text-[11px] font-bold text-background">
            F
          </div>
          {!collapsed && (
            <span className="ml-2.5 truncate text-sm font-semibold tracking-tight text-foreground">
              Flow
            </span>
          )}
        </div>
        <button
          type="button"
          onClick={toggle}
          aria-label={collapsed ? "展开侧栏" : "折叠侧栏"}
          title={collapsed ? "展开侧栏" : "折叠侧栏"}
          className="flex h-8 w-8 shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors duration-150 hover:bg-sidebar-accent hover:text-foreground"
        >
          {collapsed ? (
            <ChevronRight className="h-4 w-4" />
          ) : (
            <ChevronLeft className="h-4 w-4" />
          )}
        </button>
      </div>

      <nav className="flex flex-1 flex-col gap-0.5 p-2">
        {NAV_ITEMS.map(({ to, icon: Icon, label }) => {
          const active =
            to === "/"
              ? pathname === "/"
              : pathname === to || pathname.startsWith(`${to}/`);
          const badgeCount =
            to === "/tasks"
              ? taskAnomalyCount
              : to === "/workers"
                ? workerStaleCount
                : 0;

          return (
            <Link
              key={to}
              to={to}
              title={collapsed ? label : undefined}
              className={cn(
                "relative flex cursor-pointer items-center rounded-md text-sm text-sidebar-foreground transition-colors duration-150 hover:bg-sidebar-accent hover:text-foreground",
                collapsed ? "h-9 justify-center" : "h-9 gap-2.5 px-3",
                active &&
                  "bg-sidebar-accent font-medium text-foreground nav-active-indicator",
              )}
            >
              <Icon className="h-4 w-4 shrink-0" />
              {!collapsed && <span className="truncate">{label}</span>}
              {!collapsed && badgeCount > 0 && (
                <Badge
                  variant="destructive"
                  className="ml-auto h-5 min-w-5 justify-center px-1.5 text-[10px]"
                >
                  {badgeCount > 99 ? "99+" : badgeCount}
                </Badge>
              )}
              {collapsed && badgeCount > 0 && (
                <span className="absolute right-1.5 top-1.5 h-2 w-2 rounded-full bg-destructive" />
              )}
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
