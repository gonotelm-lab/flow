import { AppShell } from "@/components/layout/app-shell";
import { NamespaceTable } from "@/components/domain/namespace-table";

type PageProps = {
  onOpenCommand?: () => void;
};

export function NamespacesPage({ onOpenCommand }: PageProps) {
  return (
    <AppShell title="命名空间" onOpenCommand={onOpenCommand}>
      <NamespaceTable />
    </AppShell>
  );
}
