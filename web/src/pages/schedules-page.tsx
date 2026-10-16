import { AppShell } from "@/components/layout/app-shell";
import { ScheduleTable } from "@/components/domain/schedule-table";

type PageProps = {
  onOpenCommand?: () => void;
};

export function SchedulesPage({ onOpenCommand }: PageProps) {
  return (
    <AppShell title="定时任务" onOpenCommand={onOpenCommand}>
      <ScheduleTable />
    </AppShell>
  );
}
