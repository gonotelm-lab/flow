import { AppShell } from "@/components/layout/app-shell";
import { NamespacePicker } from "@/components/domain/namespace-picker";

type PageProps = {
  onOpenCommand?: () => void;
};

export function HomePage({ onOpenCommand }: PageProps) {
  return (
    <AppShell title="选择命名空间" onOpenCommand={onOpenCommand}>
      <div className="mx-auto max-w-4xl">
        <NamespacePicker />
      </div>
    </AppShell>
  );
}
