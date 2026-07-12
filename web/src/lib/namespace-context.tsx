import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react";
import { useQueryClient } from "@tanstack/react-query";

const STORAGE_KEY = "flow-selected-namespace";

type NamespaceContextValue = {
  namespace: string | null;
  setNamespace: (name: string) => void;
  clearNamespace: () => void;
};

const NamespaceContext = createContext<NamespaceContextValue | null>(null);

export function NamespaceProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient();
  const [namespace, setNamespaceState] = useState<string | null>(() =>
    localStorage.getItem(STORAGE_KEY),
  );

  const setNamespace = useCallback(
    (name: string) => {
      setNamespaceState(name);
      localStorage.setItem(STORAGE_KEY, name);
      queryClient.invalidateQueries();
    },
    [queryClient],
  );

  const clearNamespace = useCallback(() => {
    setNamespaceState(null);
    localStorage.removeItem(STORAGE_KEY);
    queryClient.invalidateQueries();
  }, [queryClient]);

  const value = useMemo(
    () => ({ namespace, setNamespace, clearNamespace }),
    [namespace, setNamespace, clearNamespace],
  );

  return (
    <NamespaceContext.Provider value={value}>
      {children}
    </NamespaceContext.Provider>
  );
}

export function useNamespace() {
  const ctx = useContext(NamespaceContext);
  if (!ctx) {
    throw new Error("useNamespace must be used within NamespaceProvider");
  }
  return ctx;
}
