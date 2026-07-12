import { useState } from "react";
import { Link, useLocation } from "react-router-dom";
import {
  Boxes,
  ChevronLeft,
  ChevronRight,
  Cpu,
  Home,
  ListTodo,
  Settings,
} from "lucide-react";
import { cn } from "@/lib/utils";

const NAV_ITEMS = [
  { to: "/", icon: Home, label: "首页" },
  { to: "/tasks", icon: ListTodo, label: "任务" },
  { to: "/workers", icon: Cpu, label: "工作节点" },
  { to: "/namespaces", icon: Boxes, label: "命名空间" },
  { to: "/settings", icon: Settings, label: "设置" },
] as const;

const STORAGE_KEY = "flow-sidebar-collapsed";

export function Sidebar() {
  const { pathname } = useLocation();
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
        "flex shrink-0 flex-col border-r border-border bg-sidebar transition-[width] duration-200",
        collapsed ? "w-14" : "w-44",
      )}
    >
      <div
        className={cn(
          "flex h-12 items-center border-b border-border",
          collapsed ? "justify-center px-0" : "px-4",
        )}
      >
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary text-xs font-bold text-primary-foreground">
          F
        </div>
        {!collapsed && (
          <span className="ml-2 truncate text-sm font-semibold">Flow</span>
        )}
      </div>

      <nav className="flex flex-1 flex-col gap-1 p-2">
        {NAV_ITEMS.map(({ to, icon: Icon, label }) => {
          const active =
            to === "/"
              ? pathname === "/"
              : pathname === to || pathname.startsWith(`${to}/`);
          return (
            <Link
              key={to}
              to={to}
              title={collapsed ? label : undefined}
              className={cn(
                "flex items-center rounded-md text-sm text-sidebar-foreground transition-colors hover:bg-accent hover:text-accent-foreground",
                collapsed ? "h-9 justify-center" : "h-9 gap-2 px-3",
                active && "bg-primary/10 text-primary",
              )}
            >
              <Icon className="h-4 w-4 shrink-0" />
              {!collapsed && <span className="truncate">{label}</span>}
            </Link>
          );
        })}
      </nav>

      <div className="border-t border-border p-2">
        <button
          type="button"
          onClick={toggle}
          title={collapsed ? "展开侧栏" : "折叠侧栏"}
          className={cn(
            "flex w-full items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground",
            collapsed ? "h-9 justify-center" : "h-9 gap-2 px-3",
          )}
        >
          {collapsed ? (
            <ChevronRight className="h-4 w-4" />
          ) : (
            <>
              <ChevronLeft className="h-4 w-4 shrink-0" />
              <span className="truncate text-sm">折叠</span>
            </>
          )}
        </button>
      </div>
    </aside>
  );
}
