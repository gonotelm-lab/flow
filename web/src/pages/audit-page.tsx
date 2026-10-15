import { AppShell } from "@/components/layout/app-shell";
import { AuditTable } from "@/components/domain/audit-table";

type PageProps = {
  onOpenCommand?: () => void;
};

export function AuditPage({ onOpenCommand }: PageProps) {
  return (
    <AppShell title="审计日志" onOpenCommand={onOpenCommand}>
      <AuditTable />
    </AppShell>
  );
}
