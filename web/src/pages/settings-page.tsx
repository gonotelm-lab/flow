import { useState } from "react";
import { toast } from "sonner";
import { AppShell } from "@/components/layout/app-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { getApiBaseUrl, setApiBaseUrl } from "@/lib/settings";
import { listNamespaces } from "@/api/namespaces";

type PageProps = {
  onOpenCommand?: () => void;
};

export function SettingsPage({ onOpenCommand }: PageProps) {
  const [baseUrl, setBaseUrl] = useState(getApiBaseUrl);

  const handleSave = () => {
    setApiBaseUrl(baseUrl.trim());
    toast.success("设置已保存");
  };

  const handleTest = async () => {
    const prev = getApiBaseUrl();
    setApiBaseUrl(baseUrl.trim());
    try {
      await listNamespaces(1, 1);
      toast.success("连接成功");
    } catch (e) {
      setApiBaseUrl(prev);
      toast.error(
        e instanceof Error ? e.message : "连接失败",
      );
    }
  };

  return (
    <AppShell title="设置" onOpenCommand={onOpenCommand}>
      <div className="max-w-md space-y-4">
        <div>
          <label className="text-sm font-medium">Admin API 基础地址</label>
          <p className="mb-2 text-sm text-muted-foreground">
            留空则使用 Vite 开发代理（默认）。API 运行在其他主机时请填写完整 URL。
          </p>
          <Input
            placeholder="http://localhost:7090"
            value={baseUrl}
            onChange={(e) => setBaseUrl(e.target.value)}
          />
        </div>
        <div className="flex gap-2">
          <Button size="sm" onClick={handleSave}>
            保存
          </Button>
          <Button size="sm" variant="outline" onClick={handleTest}>
            测试连接
          </Button>
        </div>
      </div>
    </AppShell>
  );
}
