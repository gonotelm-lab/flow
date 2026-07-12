import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createNamespace,
  listNamespaces,
  updateNamespace,
} from "@/api/namespaces";

export function useNamespaces(page = 1, pageSize = 20) {
  return useQuery({
    queryKey: ["namespaces", page, pageSize],
    queryFn: () => listNamespaces(page, pageSize),
  });
}

export function useCreateNamespace() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createNamespace,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["namespaces"] }),
  });
}

export function useUpdateNamespace() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      name,
      body,
    }: {
      name: string;
      body: { description?: string; creator?: string };
    }) => updateNamespace(name, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["namespaces"] }),
  });
}
