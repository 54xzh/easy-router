export type Provider = {
  id: string;
  name: string;
  base_url: string;
  api_key?: string;
  extra_headers: Record<string, string>;
  enabled: boolean;
  created_at?: string;
  updated_at?: string;
};

export type Model = {
  internal_id: string;
  provider_id: string;
  original_id: string;
  display_name: string;
  supports_chat: boolean;
  supports_responses: boolean;
  supports_stream: boolean;
  context_length: number;
  enabled: boolean;
  auto_disabled: boolean;
  auto_disabled_reason: string;
  provider_enabled?: boolean;
};

export type ModelGroupMember = {
  model_id: string;
  position: number;
  enabled: boolean;
  model?: Model;
};

export type ModelGroup = {
  id: string;
  name: string;
  strategy: "random" | "round_robin" | "fallback";
  enabled: boolean;
  members: ModelGroupMember[];
};

export type RouteStep = {
  id?: number;
  position: number;
  target_type: "model" | "group";
  target_id: string;
  enabled: boolean;
  label?: string;
};

export type Override = {
  route_id?: string;
  target_type: "model" | "group";
  target_id: string;
  disabled: boolean;
};

export type Route = {
  id: string;
  name: string;
  enabled: boolean;
  steps: RouteStep[];
  overrides?: Override[];
};

export type ProxyKey = {
  id: string;
  name: string;
  prefix: string;
  enabled: boolean;
  created_at: string;
  last_used_at: string;
  token?: string;
};

export type AttemptLog = {
  id: number;
  request_id: string;
  position: number;
  model_id: string;
  provider_id: string;
  status: string;
  http_status: number;
  duration_ms: number;
  error: string;
};

export type RequestLog = {
  id: string;
  created_at: string;
  api: string;
  route_id: string;
  client_model: string;
  final_model: string;
  status: string;
  http_status: number;
  duration_ms: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  error: string;
  attempts: AttemptLog[];
};

export type Settings = {
  models_expose_raw?: string;
  log_retention_days?: string;
};

