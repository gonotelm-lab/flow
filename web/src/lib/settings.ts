const KEY = "flow-api-base-url";

export function getApiBaseUrl(): string {
  return localStorage.getItem(KEY) ?? "";
}

export function setApiBaseUrl(url: string) {
  localStorage.setItem(KEY, url);
}
