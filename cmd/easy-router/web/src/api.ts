export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });
  const text = await response.text();
  const data = parseResponse(text, response.headers.get("Content-Type"));
  if (!response.ok) {
    throw new Error(readError(data) ?? text.trim() ?? `请求失败：${response.status}`);
  }
  if (typeof data === "string") {
    throw new Error("响应格式不正确");
  }
  return data as T;
}

function parseResponse(text: string, contentType: string | null): unknown {
  if (!text) return null;
  if (!contentType?.includes("application/json")) return text;
  try {
    return JSON.parse(text);
  } catch {
    throw new Error("响应 JSON 格式不正确");
  }
}

function readError(data: unknown) {
  if (!data || typeof data !== "object" || !("error" in data)) return undefined;
  const error = (data as { error?: unknown }).error;
  return typeof error === "string" ? error : undefined;
}

export function post<T>(path: string, body?: unknown): Promise<T> {
  return api<T>(path, {
    method: "POST",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function put<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

export function patch<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function del<T>(path: string): Promise<T> {
  return api<T>(path, {
    method: "DELETE",
  });
}

export function enc(value: string): string {
  return encodeURIComponent(value);
}
