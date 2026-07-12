import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Command } from "cmdk";
import { Boxes, Cpu, Home, ListTodo } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";

const pages = [
  { label: "首页", path: "/", icon: Home },
  { label: "任务", path: "/tasks", icon: ListTodo },
  { label: "工作节点", path: "/workers", icon: Cpu },
  { label: "命名空间", path: "/namespaces", icon: Boxes },
];

const shortcuts = [
  { label: "打开命令菜单", keys: "⌘K" },
  { label: "切换主题", keys: "顶栏按钮" },
];

type CommandMenuProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CommandMenu({ open, onOpenChange }: CommandMenuProps) {
  const navigate = useNavigate();

  const go = (path: string) => {
    navigate(path);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="overflow-hidden p-0 sm:max-w-md">
        <DialogTitle className="sr-only">命令菜单</DialogTitle>
        <Command className="[&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground">
          <Command.Input
            placeholder="搜索页面..."
            className="flex h-11 w-full border-b border-border bg-transparent px-4 text-sm outline-none placeholder:text-muted-foreground"
          />
          <Command.List className="max-h-72 overflow-auto p-2">
            <Command.Empty className="py-8 text-center text-sm text-muted-foreground">
              无匹配结果
            </Command.Empty>
            <Command.Group heading="导航">
              {pages.map(({ label, path, icon: Icon }) => (
                <Command.Item
                  key={path}
                  value={label}
                  onSelect={() => go(path)}
                  className="flex cursor-pointer items-center gap-2.5 rounded-md px-3 py-2.5 text-sm transition-colors duration-150 aria-selected:bg-accent"
                >
                  <Icon className="h-4 w-4 text-muted-foreground" />
                  {label}
                </Command.Item>
              ))}
            </Command.Group>
            <Command.Group heading="快捷筛选">
              <Command.Item
                value="失败任务"
                onSelect={() => go("/tasks?state=FAILED")}
                className="cursor-pointer rounded-md px-3 py-2.5 text-sm transition-colors duration-150 aria-selected:bg-accent"
              >
                失败任务
              </Command.Item>
              <Command.Item
                value="待运行任务"
                onSelect={() => go("/tasks?state=INITED")}
                className="cursor-pointer rounded-md px-3 py-2.5 text-sm transition-colors duration-150 aria-selected:bg-accent"
              >
                待运行任务
              </Command.Item>
            </Command.Group>
            <Command.Group heading="快捷键">
              {shortcuts.map(({ label, keys }) => (
                <Command.Item
                  key={label}
                  value={`快捷键 ${label}`}
                  disabled
                  className="flex cursor-default items-center justify-between rounded-md px-3 py-2.5 text-sm text-muted-foreground opacity-100"
                >
                  <span>{label}</span>
                  <kbd className="kbd">{keys}</kbd>
                </Command.Item>
              ))}
            </Command.Group>
          </Command.List>
        </Command>
      </DialogContent>
    </Dialog>
  );
}

export function useCommandMenu() {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((o) => !o);
      }
    };
    document.addEventListener("keydown", down);
    return () => document.removeEventListener("keydown", down);
  }, []);

  return { open, setOpen };
}
