import { Moon, Sun } from "lucide-react";
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
    <header className="flex h-12 items-center justify-between border-b border-border px-6">
      <div className="flex items-center gap-3">
        <h1 className="text-base font-semibold">{title}</h1>
        <NamespaceSwitcher />
      </div>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          className="hidden h-8 text-xs text-muted-foreground sm:inline-flex"
          onClick={onOpenCommand}
        >
          ⌘K
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
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
