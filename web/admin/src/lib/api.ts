export type APIOptions = RequestInit & {
  rawBody?: boolean;
};

export async function apiRequest<T = any>(path: string, opts: APIOptions = {}): Promise<T> {
  const token = (localStorage.getItem("cc_admin_token") || "").trim();
  const headers = new Headers(opts.headers || {});
  if (!opts.rawBody && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (token && !headers.has("x-admin-token")) {
    headers.set("x-admin-token", token);
  }

  const resp = await fetch(path, { ...opts, headers });
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
