import { ReactNode, useEffect, useState } from "react";
import { Activity, Boxes, Cable, GitBranch, ListTree, LogOut, Logs, RefreshCw, Settings as SettingsIcon } from "lucide-react";
import { Button, Separator } from "@heroui/react";
import { api, post } from "./api";
import type { AppData, Model, ModelGroup, Provider, ProxyKey, Route, Settings, TabKey } from "./types";
import { Providers } from "./pages/Providers";
import { Models } from "./pages/Models";
import { Groups } from "./pages/Groups";
import { Routes } from "./pages/Routes";
import { RouteSwitch } from "./pages/RouteSwitch";
import { LogsView } from "./pages/LogsView";
import { SettingsView } from "./pages/SettingsView";
import { Login } from "./pages/Login";

const emptyData: AppData = {
  providers: [],
  models: [],
  groups: [],
  routes: [],
  settings: {},
  keys: [],
};

const navItems: Array<{ key: TabKey; label: string; icon: ReactNode }> = [
  { key: "switch", label: "路由开关", icon: <Cable size={17} /> },
  { key: "providers", label: "提供商配置", icon: <Boxes size={17} /> },
  { key: "models", label: "模型列表", icon: <ListTree size={17} /> },
  { key: "groups", label: "模型组", icon: <GitBranch size={17} /> },
  { key: "routes", label: "路由配置", icon: <Activity size={17} /> },
  { key: "logs", label: "日志", icon: <Logs size={17} /> },
  { key: "settings", label: "设置", icon: <SettingsIcon size={17} /> },
];

async function loadData(): Promise<AppData> {
  const [providers, models, groups, routes, settings, keys] = await Promise.all([
    api<Provider[]>("/api/admin/providers"),
    api<Model[]>("/api/admin/models"),
    api<ModelGroup[]>("/api/admin/groups"),
    api<Route[]>("/api/admin/routes"),
    api<Settings>("/api/admin/settings"),
    api<ProxyKey[]>("/api/admin/proxy-keys"),
  ]);

  return {
    providers: providers ?? [],
    models: models ?? [],
    groups: groups ?? [],
    routes: routes ?? [],
    settings: settings ?? {},
    keys: keys ?? [],
  };
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function needsLogin(message: string) {
  return message.includes("请先登录") || message.includes("401");
}

export default function App() {
  const [ready, setReady] = useState(false);
  const [authed, setAuthed] = useState(false);
  const [active, setActive] = useState<TabKey>("switch");
  const [data, setData] = useState<AppData>(emptyData);
  const [error, setError] = useState("");

  async function refresh() {
    setData(await loadData());
  }

  function handleError(error: unknown) {
    const message = errorMessage(error);
    if (needsLogin(message)) {
      setAuthed(false);
      setData(emptyData);
      setError("");
      return;
    }
    setError(message);
  }

  function handleAuthError() {
    setAuthed(false);
    setData(emptyData);
    setError("");
  }

  async function run(task: () => Promise<void>) {
    setError("");
    try {
      await task();
      await refresh();
    } catch (error) {
      handleError(error);
    }
  }

  useEffect(() => {
    let cancelled = false;

    async function boot() {
      setError("");
      try {
        await api("/api/admin/me");
      } catch (error) {
        if (!cancelled) {
          const message = errorMessage(error);
          setAuthed(false);
          setError(needsLogin(message) ? "" : message);
          setReady(true);
        }
        return;
      }

      if (cancelled) return;
      setAuthed(true);

      try {
        const nextData = await loadData();
        if (!cancelled) {
          setData(nextData);
        }
      } catch (error) {
        if (!cancelled) {
          handleError(error);
        }
      } finally {
        if (!cancelled) {
          setReady(true);
        }
      }
    }

    boot();
    return () => {
      cancelled = true;
    };
  }, []);

  async function login(username: string, password: string) {
    setError("");
    try {
      await post("/api/admin/login", { username, password });
      setAuthed(true);
      await refresh();
    } catch (error) {
      handleError(error);
    }
  }

  async function logout() {
    setError("");
    try {
      await post("/api/admin/logout");
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setAuthed(false);
      setData(emptyData);
    }
  }

  if (!ready) {
    return (
      <div className="login-page">
        <div className="surface section stack" style={{ textAlign: "center" }}>
          <div className="brand-mark" style={{ margin: "0 auto" }}>
            <Cable size={20} />
          </div>
          <div>
            <div style={{ fontWeight: 600, fontSize: 16 }}>正在加载...</div>
            <div className="muted" style={{ fontSize: 14, marginTop: 4 }}>Easy Router</div>
          </div>
        </div>
      </div>
    );
  }

  if (!authed) {
    return <Login error={error} onLogin={login} />;
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">
            <Cable size={18} />
          </div>
          <div>
            <div style={{ fontSize: 15, fontWeight: 700 }}>Easy Router</div>
            <div className="muted" style={{ fontSize: 11, fontWeight: 500 }}>
              LLM Proxy
            </div>
          </div>
        </div>

        <nav className="nav-list">
          {navItems.map((item) => (
            <button
              className="nav-button"
              data-active={active === item.key}
              key={item.key}
              onClick={() => setActive(item.key)}
            >
              {item.icon}
              {item.label}
            </button>
          ))}
        </nav>

        <div style={{ marginTop: "auto" }}>
          <Separator className="my-3" />
          <div className="muted" style={{ fontSize: 11, padding: "0 8px", fontWeight: 500 }}>
            默认地址：127.0.0.1:2778
          </div>
        </div>
      </aside>

      <main className="main">
        <div className="toolbar">
          <div>
            <h1 className="page-title">{navItems.find((item) => item.key === active)?.label}</h1>
          </div>
          <div className="row">
            <Button variant="secondary" onPress={() => run(async () => {})}>
              <RefreshCw size={16} />
              刷新
            </Button>
            <Button variant="tertiary" onPress={logout}>
              <LogOut size={16} />
              退出
            </Button>
          </div>
        </div>

        {error ? (
          <div className="surface section error" style={{ marginBottom: 16 }}>
            {error}
          </div>
        ) : null}

        {active === "switch" && <RouteSwitch data={data} run={run} />}
        {active === "providers" && <Providers data={data} run={run} />}
        {active === "models" && <Models data={data} run={run} />}
        {active === "groups" && <Groups data={data} run={run} />}
        {active === "routes" && <Routes data={data} run={run} />}
        {active === "logs" && <LogsView routes={data.routes} onAuthError={handleAuthError} />}
        {active === "settings" && <SettingsView data={data} run={run} />}
      </main>
    </div>
  );
}
