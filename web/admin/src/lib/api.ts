export type ScopeMode = "project" | "global";

export type APIOptions = RequestInit & {
  rawBody?: boolean;
};

const TOKEN_STORAGE_KEY = "cc_admin_token";
const SCOPE_STORAGE_KEY = "cc_scope";
const PROJECT_STORAGE_KEY = "cc_project_id";
const DEFAULT_PROJECT_ID = "default";

const SCOPED_PATH_PREFIXES = [
  "/admin/tools",
  "/admin/bootstrap/apply",
  "/admin/marketplace/cloud/install",
  "/v1/cc/plugins",
  "/v1/cc/mcp/servers",
  "/v1/cc/marketplace"
];

export function normalizeProjectID(raw: string): string {
  const source = (raw || "").toLowerCase().trim();
  if (!source) {
    return DEFAULT_PROJECT_ID;
  }
  const normalized = source.replace(/[^a-z0-9._-]/g, "").slice(0, 64);
  return normalized || DEFAULT_PROJECT_ID;
}

export function getStoredScope(): ScopeMode {
  const raw = (localStorage.getItem(SCOPE_STORAGE_KEY) || "").toLowerCase().trim();
  return raw === "global" ? "global" : "project";
}

export function getStoredProjectID(): string {
  return normalizeProjectID(localStorage.getItem(PROJECT_STORAGE_KEY) || DEFAULT_PROJECT_ID);
}

export function saveStoredScope(scope: ScopeMode, projectID: string): { scope: ScopeMode; projectID: string } {
  const nextScope: ScopeMode = scope === "global" ? "global" : "project";
  const nextProjectID = normalizeProjectID(projectID);
  localStorage.setItem(SCOPE_STORAGE_KEY, nextScope);
  localStorage.setItem(PROJECT_STORAGE_KEY, nextProjectID);
  return { scope: nextScope, projectID: nextProjectID };
}

function shouldAttachScope(pathname: string): boolean {
  return SCOPED_PATH_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}

export function withScope(path: string, scope = getStoredScope(), projectID = getStoredProjectID()): string {
  const url = new URL(path, window.location.origin);
  url.searchParams.set("scope", scope);
  url.searchParams.set("project_id", normalizeProjectID(projectID));
  return `${url.pathname}${url.search}${url.hash}`;
}

function resolveRequestPath(path: string): string {
  const url = new URL(path, window.location.origin);
  if (shouldAttachScope(url.pathname)) {
    const scope = getStoredScope();
    const projectID = getStoredProjectID();
    if (!url.searchParams.has("scope")) {
      url.searchParams.set("scope", scope);
    }
    if (!url.searchParams.has("project_id")) {
      url.searchParams.set("project_id", projectID);
    }
  }
  return `${url.pathname}${url.search}${url.hash}`;
}

export async function apiRequest<T = any>(path: string, opts: APIOptions = {}): Promise<T> {
  const token = (localStorage.getItem(TOKEN_STORAGE_KEY) || "").trim();
  const headers = new Headers(opts.headers || {});
  if (!opts.rawBody && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (token && !headers.has("x-admin-token")) {
    headers.set("x-admin-token", token);
  }
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  if (!headers.has("x-project-id")) {
    headers.set("x-project-id", getStoredProjectID());
  }

  const resp = await fetch(resolveRequestPath(path), { ...opts, headers });
  if (!resp.ok) {
    const text = (await resp.text()).trim();
    throw new Error(text || `HTTP ${resp.status}`);
  }
  if (resp.status === 204) {
    return null as T;
  }

  const contentType = (resp.headers.get("content-type") || "").toLowerCase();
  if (contentType.includes("application/json")) {
    return (await resp.json()) as T;
  }
  return (await resp.text()) as T;
}

export function encodeKey(v: string): string {
  return encodeURIComponent(v || "");
}

export function decodeKey(v: string): string {
  return decodeURIComponent(v || "");
}
