import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, RefreshCw, Search, X } from "lucide-react";
import { Button, Chip } from "@heroui/react";
import { api } from "../api";
import { AttemptLog, LogCursor, LogPage, RequestLog, Route } from "../types";
import { formatTime, formatSeconds, logRouteName } from "../utils";
import { Status } from "../components/Status";
import { AppSelect, SelectOption } from "../components/AppSelect";
import { LabeledSwitch } from "../components/LabeledSwitch";

const STATUS_OPTIONS: SelectOption[] = [
  { id: "", label: "全部状态" },
  { id: "success", label: "成功" },
  { id: "failed", label: "失败" },
  { id: "stream_error", label: "流式错误" },
  { id: "canceled", label: "已取消" },
  { id: "error", label: "错误" },
];

const PAGE_SIZE = 50;

type Filters = {
  status: string;
  model: string;
  q: string;
  after: string;
  before: string;
};

function buildQuery(filters: Filters, cursor?: LogCursor | null, limit = PAGE_SIZE): string {
  const params = new URLSearchParams();
  params.set("limit", String(limit));
  if (filters.status) params.set("status", filters.status);
  if (filters.model) params.set("model", filters.model);
  if (filters.q) params.set("q", filters.q);
  if (filters.after) params.set("after", filters.after);
  if (filters.before) params.set("before", filters.before);
  if (cursor) {
    params.set("cursor_created_at", cursor.created_at);
    params.set("cursor_id", cursor.id);
  }
  return params.toString();
}

function isAuthError(message: string) {
  return message.includes("请先登录") || message.includes("401");
}

function attemptStatusLabel(status: string) {
  switch (status) {
    case "success":
      return "成功";
    case "failed":
      return "失败";
    case "canceled":
      return "已取消";
    default:
      return status || "-";
  }
}

function LogsView({ routes, onAuthError }: { routes: Route[]; onAuthError: () => void }) {
  const [filters, setFilters] = useState<Filters>({ status: "", model: "", q: "", after: "", before: "" });
  const [items, setItems] = useState<RequestLog[]>([]);
  const [nextCursor, setNextCursor] = useState<LogCursor | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [refreshInterval, setRefreshInterval] = useState(10);

  const fetchFirst = useCallback(
    async (silent = false) => {
      if (!silent) setLoading(true);
      if (!silent) setError("");
      try {
        const page = await api<LogPage>(`/api/admin/logs?${buildQuery(filters)}`);
        setItems(page.items ?? []);
        setNextCursor(page.next_cursor ?? null);
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        if (isAuthError(message)) {
          onAuthError();
          return;
        }
        setError(message);
      } finally {
        if (!silent) setLoading(false);
      }
    },
    [filters, onAuthError],
  );

  const fetchMore = useCallback(async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    try {
      const page = await api<LogPage>(`/api/admin/logs?${buildQuery(filters, nextCursor)}`);
      setItems((prev) => [...prev, ...(page.items ?? [])]);
      setNextCursor(page.next_cursor ?? null);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      if (isAuthError(message)) {
        onAuthError();
        return;
      }
      setError(message);
    } finally {
      setLoadingMore(false);
    }
  }, [filters, nextCursor, loadingMore, onAuthError]);

  useEffect(() => {
    fetchFirst();
  }, [fetchFirst]);

  useEffect(() => {
    if (!autoRefresh) return;
    const id = window.setInterval(() => {
      fetchFirst(true);
    }, refreshInterval * 1000);
    return () => window.clearInterval(id);
  }, [autoRefresh, refreshInterval, fetchFirst]);

  const stats = useMemo(() => {
    const total = items.length;
    const success = items.filter((l) => l.status === "success").length;
    const successRate = total ? Math.round((success / total) * 100) : 0;
    const totalTokens = items.reduce((sum, l) => sum + (l.total_tokens || 0), 0);
    const avgDuration = total
      ? Math.round(items.reduce((sum, l) => sum + (l.duration_ms || 0), 0) / total)
      : 0;
    return { total, successRate, totalTokens, avgDuration };
  }, [items]);

  const modelOptions = useMemo(() => {
    const seen = new Set<string>();
    items.forEach((l) => {
      if (l.client_model) seen.add(l.client_model);
    });
    return Array.from(seen).sort();
  }, [items]);

  const hasFilters = filters.status || filters.model || filters.q || filters.after || filters.before;

  function clearFilters() {
    setFilters({ status: "", model: "", q: "", after: "", before: "" });
  }

  function toggleExpand(id: string) {
    setExpanded((prev) => ({ ...prev, [id]: !prev[id] }));
  }

  return (
    <div className="surface log-view">
      <div className="log-filters">
        <AppSelect
          label="状态"
          value={filters.status}
          onChange={(value) => setFilters((f) => ({ ...f, status: value }))}
          options={STATUS_OPTIONS}
          className="log-filter-select"
        />
        <div className="log-field">
          <label className="log-field-label">客户端模型</label>
          <input
            className="log-input"
            list="log-model-options"
            placeholder="按客户端模型过滤"
            value={filters.model}
            onChange={(e) => setFilters((f) => ({ ...f, model: e.target.value }))}
          />
          <datalist id="log-model-options">
            {modelOptions.map((m) => (
              <option key={m} value={m} />
            ))}
          </datalist>
        </div>
        <div className="log-field">
          <label className="log-field-label">关键词</label>
          <input
            className="log-input"
            placeholder="搜索错误 / 模型"
            value={filters.q}
            onChange={(e) => setFilters((f) => ({ ...f, q: e.target.value }))}
          />
        </div>
        <div className="log-field">
          <label className="log-field-label">开始时间</label>
          <input
            type="datetime-local"
            className="log-input"
            value={filters.after}
            onChange={(e) => setFilters((f) => ({ ...f, after: e.target.value }))}
          />
        </div>
        <div className="log-field">
          <label className="log-field-label">结束时间</label>
          <input
            type="datetime-local"
            className="log-input"
            value={filters.before}
            onChange={(e) => setFilters((f) => ({ ...f, before: e.target.value }))}
          />
        </div>
        <div className="log-filter-actions">
          <Button variant="secondary" onPress={() => fetchFirst()} isDisabled={loading}>
            <Search size={15} />
            筛选
          </Button>
          {hasFilters ? (
            <Button variant="ghost" onPress={clearFilters}>
              <X size={15} />
              清除
            </Button>
          ) : null}
        </div>
      </div>

      <div className="log-toolbar">
        <div className="log-stats">
          <span className="log-stat">
            <span className="muted">条数</span>
            <b>{stats.total}</b>
          </span>
          <span className="log-stat">
            <span className="muted">成功率</span>
            <b>{stats.successRate}%</b>
          </span>
          <span className="log-stat">
            <span className="muted">平均耗时</span>
            <b>{formatSeconds(stats.avgDuration)}</b>
          </span>
          <span className="log-stat">
            <span className="muted">总 Tokens</span>
            <b>{stats.totalTokens}</b>
          </span>
        </div>
        <div className="log-toolbar-right">
          <div className="log-autorefresh">
            <LabeledSwitch
              compact
              label="自动刷新"
              selected={autoRefresh}
              onChange={setAutoRefresh}
            />
            {autoRefresh ? (
              <select
                className="log-input log-interval-select"
                value={refreshInterval}
                onChange={(e) => setRefreshInterval(Number(e.target.value) || 10)}
              >
                <option value={5}>5 秒</option>
                <option value={10}>10 秒</option>
                <option value={30}>30 秒</option>
                <option value={60}>60 秒</option>
              </select>
            ) : null}
          </div>
          <Button variant="secondary" onPress={() => fetchFirst()} isDisabled={loading}>
            <RefreshCw size={15} className={loading ? "spin" : ""} />
            刷新
          </Button>
        </div>
      </div>

      {error ? <div className="log-error">{error}</div> : null}

      {loading && items.length === 0 ? (
        <div className="log-empty muted">加载中…</div>
      ) : items.length === 0 ? (
        <div className="log-empty muted">暂无日志</div>
      ) : (
        <div className="table-wrap">
          <table className="data-table log-table">
            <thead>
              <tr>
                <th className="log-chevron-col" />
                <th>时间</th>
                <th>接口</th>
                <th>模型</th>
                <th>路由</th>
                <th>状态</th>
                <th>总耗时</th>
                <th>首字</th>
                <th>Tokens</th>
              </tr>
            </thead>
            <tbody>
              {items.map((log) => {
                const isOpen = !!expanded[log.id];
                return (
                  <LogRow
                    key={log.id}
                    log={log}
                    routes={routes}
                    isOpen={isOpen}
                    onToggle={() => toggleExpand(log.id)}
                  />
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {items.length > 0 ? (
        <div className="log-loadmore">
          {nextCursor ? (
            <Button variant="secondary" onPress={fetchMore} isDisabled={loadingMore}>
              {loadingMore ? "加载中…" : "加载更多"}
            </Button>
          ) : (
            <span className="muted">没有更多了</span>
          )}
        </div>
      ) : null}
    </div>
  );
}

function LogRow({
  log,
  routes,
  isOpen,
  onToggle,
}: {
  log: RequestLog;
  routes: Route[];
  isOpen: boolean;
  onToggle: () => void;
}) {
  return (
    <>
      <tr className={isOpen ? "log-row log-row-open" : "log-row"} onClick={onToggle}>
        <td className="log-chevron-col">
          {isOpen ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
        </td>
        <td className="log-time">{formatTime(log.created_at)}</td>
        <td>{log.api}</td>
        <td>
          <div className="code">{log.client_model}</div>
          <div className="muted">最终：{log.final_model || "-"}</div>
        </td>
        <td className="muted">{logRouteName(log, routes)}</td>
        <td>
          <Status ok={log.status === "success"} text={logStatusLabel(log.status)} />
        </td>
        <td>{formatSeconds(log.duration_ms)}</td>
        <td>{formatSeconds(log.first_token_ms)}</td>
        <td>{log.total_tokens || "-"}</td>
      </tr>
      {isOpen ? (
        <tr className="log-detail-row">
          <td colSpan={9}>
            <LogDetail log={log} routes={routes} />
          </td>
        </tr>
      ) : null}
    </>
  );
}

function LogDetail({ log, routes }: { log: RequestLog; routes: Route[] }) {
  return (
    <div className="log-detail">
      <div className="log-detail-meta">
        <span className="muted">请求 ID：</span>
        <span className="code">{log.id}</span>
      </div>
      <div className="log-detail-meta">
        <span className="muted">路由：</span>
        <span>{logRouteName(log, routes)}</span>
        <span className="muted" style={{ marginLeft: 16 }}>HTTP：</span>
        <span>{log.http_status || "-"}</span>
        <span className="muted" style={{ marginLeft: 16 }}>总耗时：</span>
        <span>{formatSeconds(log.duration_ms)}</span>
        <span className="muted" style={{ marginLeft: 16 }}>首字：</span>
        <span>{formatSeconds(log.first_token_ms)}</span>
      </div>
      <div className="log-detail-meta">
        <span className="muted">Tokens：</span>
        <span>
          {log.total_tokens || 0}（提示 {log.prompt_tokens || 0} / 补全 {log.completion_tokens || 0}）
        </span>
      </div>
      {log.error ? (
        <div className="log-detail-error">
          <span className="muted">错误：</span>
          <span className="error">{log.error}</span>
        </div>
      ) : null}

      <div className="log-detail-section-label muted">尝试（{log.attempts?.length ?? 0}）</div>
      {log.attempts && log.attempts.length > 0 ? (
        <div className="log-timeline">
          {log.attempts.map((attempt) => (
            <AttemptRow key={attempt.id} attempt={attempt} />
          ))}
        </div>
      ) : (
        <div className="muted">无尝试记录</div>
      )}
    </div>
  );
}

function AttemptRow({ attempt }: { attempt: AttemptLog }) {
  return (
    <div className="log-attempt">
      <div className="log-attempt-head">
        <span className="log-attempt-pos">#{attempt.position}</span>
        <span className="code">{attempt.model_id}</span>
        {attempt.key_name ? (
          <Chip size="sm" variant="secondary">
            {attempt.key_name}
            {attempt.key_prefix ? ` · ${attempt.key_prefix}...` : ""}
          </Chip>
        ) : null}
        <Status ok={attempt.status === "success"} text={attemptStatusLabel(attempt.status)} />
        <span className="muted">HTTP {attempt.http_status || "-"}</span>
        <span className="muted">总耗时 {formatSeconds(attempt.duration_ms)}</span>
        <span className="muted">首字 {formatSeconds(attempt.first_token_ms)}</span>
      </div>
      {attempt.error ? <div className="error log-attempt-error">{attempt.error}</div> : null}
    </div>
  );
}

function logStatusLabel(status: string) {
  switch (status) {
    case "success":
      return "成功";
    case "failed":
      return "失败";
    case "stream_error":
      return "流式错误";
    case "canceled":
      return "已取消";
    default:
      return status || "错误";
  }
}

export { LogsView };
