import { AppShell } from "@/components/layout/app-shell";
import { WorkerTable } from "@/components/domain/worker-table";

type PageProps = {
  onOpenCommand?: () => void;
};

export function WorkersPage({ onOpenCommand }: PageProps) {
  return (
    <AppShell title="工作节点" onOpenCommand={onOpenCommand}>
      <WorkerTable />
    </AppShell>
  );
}
