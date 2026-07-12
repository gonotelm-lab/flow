import { Sidebar } from "./sidebar";
import { Topbar } from "./topbar";

type AppShellProps = {
  title: string;
  children: React.ReactNode;
  onOpenCommand?: () => void;
};

export function AppShell({ title, children, onOpenCommand }: AppShellProps) {
  return (
    <div className="flex h-screen bg-background text-foreground">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <Topbar title={title} onOpenCommand={onOpenCommand} />
        <main className="flex-1 overflow-auto p-6">{children}</main>
      </div>
    </div>
  );
}
