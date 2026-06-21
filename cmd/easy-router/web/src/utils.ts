import { Model, ModelGroup, Provider, Route, RouteStep, Settings, AppData, RequestLog } from "./types";
import { RouteStepForm } from "./pages/Routes";
import { GroupMemberForm } from "./pages/Groups";

export function moveItem<T>(items: T[], index: number, delta: number) {
  const nextIndex = index + delta;
  if (nextIndex < 0 || nextIndex >= items.length) return items;
  const next = [...items];
  const [item] = next.splice(index, 1);
  next.splice(nextIndex, 0, item);
  return next;
}

export function formatTime(value: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

export async function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  const ok = document.execCommand("copy");
  document.body.removeChild(textarea);
  if (!ok) {
    throw new Error("copy failed");
  }
}

export function strategyLabel(strategy: ModelGroup["strategy"]) {
  switch (strategy) {
    case "random":
      return "随机";
    case "round_robin":
      return "轮询";
    case "fallback":
    default:
      return "固定顺序";
  }
}

export function modelName(models: Model[], id: string) {
  return id;
}

export function groupName(groups: ModelGroup[], id: string) {
  const group = groups.find((item) => item.id === id);
  return group?.name || id;
}

export function routeModelStateLabel(model: Model | undefined, providers: Provider[], autoDisableOn: boolean) {
  if (!model) return "";
  if (!routeModelProviderEnabled(model, providers)) return "提供商未启用";
  if (!model.enabled) return "未启用";
  if (autoDisableOn && model.auto_disabled) return "已禁用";
  if (autoDisableOn && modelCoolingDown(model)) return "冷却中";
  return "";
}

export function routeMemberStateLabel(
  member: GroupMemberForm,
  model: Model | undefined,
  providers: Provider[],
  autoDisableOn: boolean,
) {
  if (!member.enabled) return "未启用";
  return routeModelStateLabel(model, providers, autoDisableOn);
}

export function routeModelProviderEnabled(model: Model, providers: Provider[]) {
  const provider = providers.find((item) => item.id === model.provider_id);
  if (provider) return provider.enabled;
  return model.provider_enabled !== false;
}

export function routeGroupStateLabel(group: ModelGroup | undefined) {
  if (!group) return "";
  return group.enabled ? "" : "未启用";
}

export function targetKey(type: "model" | "group", id: string) {
  return `${type}:${id}`;
}

export function routeOverrideDisabled(route: Route | undefined, type: "model" | "group", id: string) {
  return route?.overrides?.some((item) => item.target_type === type && item.target_id === id && item.disabled) ?? false;
}

export function routeStepEnabled(route: Route | undefined, step: Pick<RouteStep, "target_type" | "target_id" | "enabled">) {
  return step.enabled && !routeOverrideDisabled(route, step.target_type, step.target_id);
}

export function routeClosedCount(route: Route) {
  const closedTargets = new Set(
    route.steps.filter((step) => !routeStepEnabled(route, step)).map((step) => targetKey(step.target_type, step.target_id)),
  );
  const overrideOnly =
    route.overrides?.filter((item) => item.disabled && !closedTargets.has(targetKey(item.target_type, item.target_id)))
      .length ?? 0;
  return closedTargets.size + overrideOnly;
}

export function routeTargetID(type: "model" | "group", target?: ModelGroup | Model) {
  if (!target) return "";
  return type === "group" ? (target as ModelGroup).id : (target as Model).internal_id;
}

export function routeTargetName(type: "model" | "group", target: ModelGroup | Model) {
  return type === "group"
    ? (target as ModelGroup).name || (target as ModelGroup).id
    : (target as Model).internal_id;
}

export function routeStepName(data: AppData, step: RouteStepForm) {
  return step.target_type === "group"
    ? groupName(data.groups, step.target_id)
    : modelName(data.models, step.target_id);
}

export function autoDisableEnabled(settings: Settings) {
  return settings.auto_disable_models !== "false";
}

export const modelCooldownLimit = 3;

export function modelCooldownLabel(model: Model) {
  const count = model.cooldown_count ?? 0;
  if (count <= 0) return "";
  const label = `冷却 ${Math.min(count, modelCooldownLimit)}/${modelCooldownLimit}`;
  if (!modelCoolingDown(model)) return label;
  return `${label} · ${modelCooldownRemaining(model)}`;
}

export function modelCoolingDown(model: Model) {
  const until = Date.parse(model.cooldown_until || "");
  return Number.isFinite(until) && until > Date.now();
}

export function modelCooldownRemaining(model: Model) {
  const until = Date.parse(model.cooldown_until || "");
  const minutes = Math.max(1, Math.ceil((until - Date.now()) / 60000));
  return `${minutes}m`;
}

export function modelHasAutoState(model: Model) {
  return model.auto_disabled || (model.cooldown_count ?? 0) > 0 || Boolean(model.cooldown_until);
}

export function modelHasClearableState(model: Model) {
  return modelHasAutoState(model) || (model.upstream_error_status ?? 0) > 0;
}

export function modelIssueLabel(model: Model) {
  switch (model.upstream_error_status) {
    case 401:
      return "密钥异常 401";
    case 403:
      return "权限受限 403";
    case 404:
      return "模型或接口不存在 404";
    case 405:
      return "接口不支持 405";
    case 410:
      return "模型已下线 410";
    default:
      return model.upstream_error_status > 0 ? `上游 ${model.upstream_error_status}` : "";
  }
}

export function modelIssueTitle(model: Model) {
  const parts = [
    model.upstream_error_at ? `时间：${formatTime(model.upstream_error_at)}` : "",
    model.upstream_error,
  ].filter(Boolean);
  return parts.join("\n");
}

export function replaceItem<T>(items: T[], index: number, item: T) {
  return items.map((current, currentIndex) => (currentIndex === index ? item : current));
}

export function removeItem<T>(items: T[], index: number) {
  return items.filter((_, currentIndex) => currentIndex !== index);
}

export function providerDeleteDescription(modelCount: number) {
  if (modelCount === 0) {
    return "删除后会移除这个提供商配置。";
  }
  return `删除后会移除这个提供商配置，并同时删除 ${modelCount} 个已添加模型。相关模型组和路由请之后检查。`;
}

export function formatSeconds(ms: number) {
  if (!ms || ms <= 0) return "-";
  return `${(ms / 1000).toFixed(2)}s`;
}

export function logRouteName(log: RequestLog, routes: Route[]) {
  if (!log.route_id) return "-";
  const route = routes.find((item) => item.id === log.route_id);
  return route ? route.name : log.route_id;
}