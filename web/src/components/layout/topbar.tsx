import { Moon, Search, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTheme } from "@/components/theme-provider";
import { NamespaceSwitcher } from "@/components/domain/namespace-switcher";

type TopbarProps = {
  title: string;
  onOpenCommand?: () => void;
};

export function Topbar({ title, onOpenCommand }: TopbarProps) {
  const { resolved, setTheme } = useTheme();

  return (
    <header className="sticky top-0 z-10 flex h-12 shrink-0 items-center justify-between border-b border-border bg-background/80 px-6 backdrop-blur-md">
      <div className="flex min-w-0 items-center gap-3">
        <h1 className="text-sm font-semibold tracking-tight">{title}</h1>
        <NamespaceSwitcher />
      </div>
      <div className="flex items-center gap-1.5">
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 sm:hidden"
          onClick={onOpenCommand}
          aria-label="打开命令菜单"
        >
          <Search className="h-4 w-4" />
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="hidden h-8 gap-1.5 px-2.5 sm:inline-flex"
          onClick={onOpenCommand}
        >
          <span className="text-xs text-muted-foreground">搜索</span>
          <kbd className="kbd">⌘K</kbd>
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 text-muted-foreground"
          aria-label={resolved === "dark" ? "切换到浅色模式" : "切换到深色模式"}
          onClick={() => setTheme(resolved === "dark" ? "light" : "dark")}
        >
          {resolved === "dark" ? (
            <Sun className="h-4 w-4" />
          ) : (
            <Moon className="h-4 w-4" />
          )}
        </Button>
      </div>
    </header>
  );
}
