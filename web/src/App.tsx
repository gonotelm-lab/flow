import { Navigate, Route, Routes } from "react-router-dom";
import {
  CommandMenu,
  useCommandMenu,
} from "@/components/domain/command-menu";
import { NamespaceRouteGuard } from "@/components/domain/namespace-route-guard";
import { HomePage } from "@/pages/home-page";
import { TasksPage } from "@/pages/tasks-page";
import { WorkersPage } from "@/pages/workers-page";
import { NamespacesPage } from "@/pages/namespaces-page";
import { SettingsPage } from "@/pages/settings-page";

export default function App() {
  const { open, setOpen } = useCommandMenu();

  return (
    <NamespaceRouteGuard>
      <Routes>
        <Route
          path="/"
          element={<HomePage onOpenCommand={() => setOpen(true)} />}
        />
        <Route
          path="/tasks"
          element={<TasksPage onOpenCommand={() => setOpen(true)} />}
        />
        <Route
          path="/workers"
          element={<WorkersPage onOpenCommand={() => setOpen(true)} />}
        />
        <Route
          path="/namespaces"
          element={<NamespacesPage onOpenCommand={() => setOpen(true)} />}
        />
        <Route
          path="/settings"
          element={<SettingsPage onOpenCommand={() => setOpen(true)} />}
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
      <CommandMenu open={open} onOpenChange={setOpen} />
    </NamespaceRouteGuard>
  );
}
