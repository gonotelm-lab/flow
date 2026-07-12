import { AppShell } from "@/components/layout/app-shell";
import { TaskTable } from "@/components/domain/task-table";

type PageProps = {
  onOpenCommand?: () => void;
};

export function TasksPage({ onOpenCommand }: PageProps) {
  return (
    <AppShell title="任务" onOpenCommand={onOpenCommand}>
      <TaskTable />
    </AppShell>
  );
}
