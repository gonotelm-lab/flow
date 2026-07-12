import { apiFetch, buildQuery, pageQueryParams } from "./client";
import type { ListNamespacesResponse, Namespace } from "./types";

export function listNamespaces(page = 1, pageSize = 20) {
  return apiFetch<ListNamespacesResponse>(
    `/api/admin/v1/namespaces${buildQuery(pageQueryParams(page, pageSize))}`,
  );
}

export function getNamespace(name: string) {
  return apiFetch<Namespace>(`/api/admin/v1/namespaces/${encodeURIComponent(name)}`);
}

export function createNamespace(body: {
  namespace: { name: string; description?: string; creator?: string };
}) {
  return apiFetch<Namespace>("/api/admin/v1/namespaces", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function updateNamespace(
  name: string,
  body: { description?: string; creator?: string },
) {
  return apiFetch<Namespace>(`/api/admin/v1/namespaces/${encodeURIComponent(name)}`, {
    method: "PUT",
    body: JSON.stringify({ name, ...body }),
  });
}
