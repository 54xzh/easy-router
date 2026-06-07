import { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";
import {
  Background,
  Controls,
  Edge,
  MarkerType,
  Node,
  Position,
  ReactFlow,
} from "@xyflow/react";
import {
  Activity,
  ArrowDown,
  ArrowUp,
  Boxes,
  Cable,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  Download,
  Edit3,
  Eye,
  EyeOff,
  GitBranch,
  KeyRound,
  ListTree,
  Logs,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Settings as SettingsIcon,
  Shield,
  Trash2,
} from "lucide-react";
import {
  Button,
  Description,
  Input,
  Label,
  ListBox,
  Modal,
  Select,
  Switch,
  TextArea,
  TextField,
} from "@heroui/react";
import { api, del, enc, patch, post, put } from "./api";
import type {
  Model,
  ModelGroup,
  ModelGroupMember,
  Provider,
  ProxyKey,
  RequestLog,
  Route,
  RouteStep,
  Settings,
} from "./types";

type TabKey = "switch" | "providers" | "models" | "groups" | "routes" | "logs" | "settings";

type AppData = {
  providers: Provider[];
  models: Model[];
  groups: ModelGroup[];
  routes: Route[];
  logs: RequestLog[];
  settings: Settings;
  keys: ProxyKey[];
};

type RemoteModel = {
  original_id: string;
  display_name: string;
  internal_id: string;
  already_imported: boolean;
  supports_chat: boolean;
  supports_responses: boolean;
  supports_stream: boolean;
};

type SelectOption = {
  id: string;
  label: ReactNode;
  textValue?: string;
};

const emptyData: AppData = {
  providers: [],
  models: [],
  groups: [],
  routes: [],
  logs: [],
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

export default function App() {
  const [ready, setReady] = useState(false);
  const [authed, setAuthed] = useState(false);
  const [active, setActive] = useState<TabKey>("switch");
  const [data, setData] = useState<AppData>(emptyData);
  const [error, setError] = useState("");

  async function refresh() {
    const [providers, models, groups, routes, logs, settings, keys] = await Promise.all([
      api<Provider[]>("/api/admin/providers"),
      api<Model[]>("/api/admin/models"),
      api<ModelGroup[]>("/api/admin/groups"),
      api<Route[]>("/api/admin/routes"),
      api<RequestLog[]>("/api/admin/logs?limit=100"),
      api<Settings>("/api/admin/settings"),
      api<ProxyKey[]>("/api/admin/proxy-keys"),
    ]);
    setData({
      providers: providers ?? [],
      models: models ?? [],
      groups: groups ?? [],
      routes: routes ?? [],
      logs: logs ?? [],
      settings: settings ?? {},
      keys: keys ?? [],
    });
  }

  useEffect(() => {
    api("/api/admin/me")
      .then(async () => {
        setAuthed(true);
        await refresh();
      })
      .catch(() => setAuthed(false))
      .finally(() => setReady(true));
  }, []);

  async function run(task: () => Promise<void>) {
    setError("");
    try {
      await task();
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  if (!ready) {
    return <div className="login-page muted">正在加载...</div>;
  }

  if (!authed) {
    return (
      <Login
        error={error}
        onLogin={async (username, password) => {
          await run(async () => {
            await post("/api/admin/login", { username, password });
            setAuthed(true);
            await refresh();
          });
        }}
      />
    );
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">
            <Cable size={18} />
          </div>
          <div>
            <div>Easy Router</div>
            <div className="muted" style={{ fontSize: 12 }}>
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
        <div className="muted" style={{ marginTop: "auto", fontSize: 12 }}>
          默认地址：127.0.0.1:2778
        </div>
      </aside>

      <main className="main">
        <div className="toolbar">
          <div>
            <h1 className="page-title">{navItems.find((item) => item.key === active)?.label}</h1>

          </div>
          <div className="row">
            <Button variant="secondary" onPress={() => run(refresh)}>
              <RefreshCw size={16} />
              刷新
            </Button>
            <Button
              variant="tertiary"
              onPress={() =>
                run(async () => {
                  await post("/api/admin/logout");
                  setAuthed(false);
                })
              }
            >
              退出
            </Button>
          </div>
        </div>

        {error ? <div className="section surface error">{error}</div> : null}

        {active === "switch" && <RouteSwitch data={data} run={run} />}
        {active === "providers" && <Providers data={data} run={run} />}
        {active === "models" && <Models data={data} run={run} />}
        {active === "groups" && <Groups data={data} run={run} />}
        {active === "routes" && <Routes data={data} run={run} />}
        {active === "logs" && <LogsView logs={data.logs} />}
        {active === "settings" && <SettingsView data={data} run={run} />}
      </main>
    </div>
  );
}



function Login({
  error,
  onLogin,
}: {
  error: string;
  onLogin: (username: string, password: string) => Promise<void>;
}) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");

  return (
    <div className="login-page">
      <form
        className="login-box surface section stack"
        onSubmit={(event) => {
          event.preventDefault();
          onLogin(username, password);
        }}
      >
        <div>
          <h1 className="page-title">登录 Easy Router</h1>
          <div className="page-subtitle">首次启动密码会显示在后端控制台。</div>
        </div>
        {error ? <div className="error">{error}</div> : null}
        <TextField fullWidth value={username} onChange={setUsername}>
          <Label>用户名</Label>
          <Input placeholder="admin" />
        </TextField>
        <TextField fullWidth type="password" value={password} onChange={setPassword}>
          <Label>密码</Label>
          <Input placeholder="输入管理员密码" />
        </TextField>
        <Button type="submit">
          <Shield size={16} />
          登录
        </Button>
      </form>
    </div>
  );
}

function Providers({ data, run }: { data: AppData; run: (task: () => Promise<void>) => void }) {
  const blankProvider: Provider = {
    id: "",
    name: "",
    base_url: "",
    api_key: "",
    extra_headers: {},
    enabled: true,
  };
  const [form, setForm] = useState<Provider>(blankProvider);
  const [headersText, setHeadersText] = useState("{}");
  const [editing, setEditing] = useState(false);
  const [expanded, setExpanded] = useState("");
  const [remoteModels, setRemoteModels] = useState<Record<string, RemoteModel[]>>({});
  const [selectedRemote, setSelectedRemote] = useState<Record<string, string[]>>({});
  const [manualModel, setManualModel] = useState<Record<string, string>>({});
  const autoDisableOn = autoDisableEnabled(data.settings);

  const modelsByProvider = useMemo(() => {
    const map = new Map<string, Model[]>();
    data.models.forEach((model) => {
      const items = map.get(model.provider_id) ?? [];
      items.push(model);
      map.set(model.provider_id, items);
    });
    return map;
  }, [data.models]);

  function openAdd() {
    setForm(blankProvider);
    setHeadersText("{}");
    setEditing(true);
  }

  function openEdit(provider: Provider) {
    setForm({ ...provider, api_key: "" });
    setHeadersText(JSON.stringify(provider.extra_headers ?? {}, null, 2));
    setEditing(true);
  }

  function closeEditor() {
    setEditing(false);
    setForm(blankProvider);
    setHeadersText("{}");
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    run(async () => {
      const headers = headersText.trim() ? JSON.parse(headersText) : {};
      const payload = { ...form, extra_headers: headers };
      if (data.providers.some((provider) => provider.id === form.id)) {
        await put(`/api/admin/providers/${enc(form.id)}`, payload);
      } else {
        await post("/api/admin/providers", payload);
      }
      closeEditor();
    });
  }

  function toggleRemote(providerID: string, originalID: string) {
    setSelectedRemote((current) => {
      const selected = new Set(current[providerID] ?? []);
      if (selected.has(originalID)) {
        selected.delete(originalID);
      } else {
        selected.add(originalID);
      }
      return { ...current, [providerID]: Array.from(selected) };
    });
  }

  return (
    <div className="surface">
      <div className="section row" style={{ justifyContent: "space-between" }}>
        <div>
          <h2 style={{ margin: 0 }}>提供商</h2>

        </div>
        <Button onPress={openAdd}>
          <Plus size={16} />
          添加提供商
        </Button>
      </div>

      <div className="provider-list">
        {data.providers.length === 0 ? (
          <div className="section muted">暂无提供商</div>
        ) : null}
        {data.providers.map((provider) => {
          const providerModels = modelsByProvider.get(provider.id) ?? [];
          const isOpen = expanded === provider.id;
          const discovered = remoteModels[provider.id] ?? [];
          const selected = selectedRemote[provider.id] ?? [];
          return (
            <div className="provider-item" key={provider.id}>
              <div className="provider-header">
                <button
                  className="icon-button"
                  onClick={() => setExpanded(isOpen ? "" : provider.id)}
                  title={isOpen ? "收起" : "展开"}
                >
                  {isOpen ? <ChevronDown size={18} /> : <ChevronRight size={18} />}
                </button>
                <div className="provider-title">
                  <strong>{provider.name || provider.id}</strong>
                  <span className="code">{provider.id}</span>
                  <span className="muted">{provider.base_url}</span>
                </div>
                <div className="row provider-actions">
                  {provider.enabled ? <Status ok text="启用" /> : <Status text="禁用" />}
                  <Button size="sm" variant="secondary" onPress={() => openEdit(provider)}>
                    <Edit3 size={14} />
                    编辑
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    onPress={() => run(async () => del(`/api/admin/providers/${enc(provider.id)}`))}
                  >
                    <Trash2 size={14} />
                  </Button>
                </div>
              </div>

              {isOpen ? (
                <div className="provider-detail">
                  <div className="row" style={{ justifyContent: "space-between" }}>
                    <h3 style={{ margin: 0 }}>已添加模型</h3>
                    <div className="row">
                      <input
                        value={manualModel[provider.id] ?? ""}
                        onChange={(event) =>
                          setManualModel({ ...manualModel, [provider.id]: event.target.value })
                        }
                        placeholder="手动输入原模型 ID"
                      />
                      <Button
                        size="sm"
                        variant="secondary"
                        onPress={() =>
                          run(async () => {
                            const originalID = (manualModel[provider.id] ?? "").trim();
                            if (!originalID) {
                              throw new Error("请先输入模型 ID");
                            }
                            await post(`/api/admin/providers/${enc(provider.id)}/models`, {
                              original_id: originalID,
                              display_name: originalID,
                              supports_chat: true,
                              supports_responses: false,
                              supports_stream: true,
                              enabled: true,
                            });
                            setManualModel({ ...manualModel, [provider.id]: "" });
                          })
                        }
                      >
                        <Plus size={14} />
                        手动添加
                      </Button>
                      <Button
                        size="sm"
                        variant="secondary"
                        onPress={() =>
                          run(async () => {
                            const models = await post<RemoteModel[]>(
                              `/api/admin/providers/${enc(provider.id)}/sync`,
                            );
                            setRemoteModels({ ...remoteModels, [provider.id]: models });
                            setSelectedRemote({ ...selectedRemote, [provider.id]: [] });
                          })
                        }
                      >
                        <RefreshCw size={14} />
                        拉取模型
                      </Button>
                    </div>
                  </div>

                  <ProviderModels models={providerModels} autoDisableEnabled={autoDisableOn} />

                  {discovered.length > 0 ? (
                    <div className="model-pick-list">
                      <div className="row" style={{ justifyContent: "space-between" }}>
                        <strong>拉取结果</strong>
                        <Button
                          size="sm"
                          onPress={() =>
                            run(async () => {
                              const models = discovered
                                .filter((model) => selected.includes(model.original_id))
                                .map((model) => ({
                                  original_id: model.original_id,
                                  display_name: model.display_name,
                                  supports_chat: model.supports_chat,
                                  supports_responses: model.supports_responses,
                                  supports_stream: model.supports_stream,
                                  enabled: true,
                                }));
                              await post(`/api/admin/providers/${enc(provider.id)}/models/import`, { models });
                              setRemoteModels({ ...remoteModels, [provider.id]: [] });
                              setSelectedRemote({ ...selectedRemote, [provider.id]: [] });
                            })
                          }
                        >
                          <Download size={14} />
                          导入选中
                        </Button>
                      </div>
                      {discovered.map((model) => {
                        const checked = selected.includes(model.original_id);
                        return (
                          <label className="model-pick-row" key={model.original_id}>
                            <input
                              type="checkbox"
                              disabled={model.already_imported}
                              checked={checked}
                              onChange={() => toggleRemote(provider.id, model.original_id)}
                            />
                            <span className="code">{model.original_id}</span>
                            {model.already_imported ? <span className="badge">已添加</span> : null}
                          </label>
                        );
                      })}
                    </div>
                  ) : null}
                </div>
              ) : null}
            </div>
          );
        })}
      </div>

      <Modal>
        <Modal.Backdrop
          isOpen={editing}
          onOpenChange={(open) => {
            if (!open) closeEditor();
          }}
          variant="blur"
        >
          <Modal.Container scroll="inside" size="lg">
            <Modal.Dialog className="config-dialog">
              <form className="modal-form" onSubmit={submit}>
                <Modal.CloseTrigger />
                <Modal.Header>
                  <Modal.Heading>
                    {data.providers.some((provider) => provider.id === form.id) ? "编辑提供商" : "添加提供商"}
                  </Modal.Heading>

                </Modal.Header>
                <Modal.Body className="config-modal-body">
                  <div className="grid-2">
                    <TextField
                      fullWidth
                      isDisabled={data.providers.some((provider) => provider.id === form.id)}
                      value={form.id}
                      onChange={(id) => setForm({ ...form, id })}
                    >
                      <Label>提供商 ID</Label>
                      <Input placeholder="openai" />
                      <Description>保存后不可修改</Description>
                    </TextField>
                    <TextField fullWidth value={form.name} onChange={(name) => setForm({ ...form, name })}>
                      <Label>显示名称</Label>
                      <Input placeholder="OpenAI" />
                    </TextField>
                    <TextField
                      fullWidth
                      value={form.base_url}
                      onChange={(base_url) => setForm({ ...form, base_url })}
                    >
                      <Label>Base URL</Label>
                      <Input placeholder="https://api.openai.com/v1" />
                    </TextField>
                    <TextField
                      fullWidth
                      type="password"
                      value={form.api_key ?? ""}
                      onChange={(api_key) => setForm({ ...form, api_key })}
                    >
                      <Label>API Key</Label>
                      <Input placeholder="编辑时留空表示不修改" />
                    </TextField>
                  </div>
                  <TextField fullWidth value={headersText} onChange={setHeadersText}>
                    <Label>额外请求头 JSON</Label>
                    <TextArea rows={3} placeholder='{"HTTP-Referer":"http://localhost"}' />
                  </TextField>
                  <LabeledSwitch
                    label="启用提供商"
                    selected={form.enabled}
                    onChange={(enabled) => setForm({ ...form, enabled })}
                  />
                </Modal.Body>
                <Modal.Footer>
                  <Button type="button" variant="tertiary" onPress={closeEditor}>
                    取消
                  </Button>
                  <Button type="submit">
                    <Save size={16} />
                    保存
                  </Button>
                </Modal.Footer>
              </form>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </div>
  );
}

function ProviderModels({
  models,
  autoDisableEnabled,
}: {
  models: Model[];
  autoDisableEnabled: boolean;
}) {
  if (models.length === 0) {
    return <div className="empty-state">暂无模型</div>;
  }
  return (
    <div className="table-wrap provider-models">
      <table className="data-table">
        <thead>
          <tr>
            <th>模型</th>
            <th>能力</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          {models.map((model) => {
            const cooldownText = autoDisableEnabled ? modelCooldownLabel(model) : "";
            const issueLabel = modelIssueLabel(model);
            return (
              <tr key={model.internal_id}>
                <td>{model.internal_id}</td>
                <td>
                  <div className="row">
                    {model.supports_chat ? <span className="badge badge-success">Chat</span> : null}
                    {model.supports_responses ? <span className="badge">Responses</span> : null}
                    {model.supports_stream ? <span className="badge">流式</span> : null}
                  </div>
                </td>
                <td>
                  <div className="stack-tight">
                    {autoDisableEnabled && model.auto_disabled ? (
                      <span className="badge badge-danger">自动禁用</span>
                    ) : cooldownText ? (
                      <span className="badge badge-warning">{cooldownText}</span>
                    ) : model.enabled ? (
                      <Status ok text="启用" />
                    ) : (
                      <Status text="禁用" />
                    )}
                    {issueLabel ? (
                      <span className="badge badge-warning" title={modelIssueTitle(model)}>
                        {issueLabel}
                      </span>
                    ) : null}
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function Models({ data, run }: { data: AppData; run: (task: () => Promise<void>) => void }) {
  const autoDisableOn = autoDisableEnabled(data.settings);

  function updateModel(model: Model, patchData: Partial<Model>) {
    run(async () => {
      await patch(`/api/admin/models/${enc(model.internal_id)}`, { ...model, ...patchData });
    });
  }

  return (
    <div className="surface table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>模型</th>
            <th>能力</th>
            <th>上下文</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {data.models.map((model) => {
            const autoDisabledActive = autoDisableOn && model.auto_disabled;
            const cooldownText = autoDisableOn ? modelCooldownLabel(model) : "";
            const issueLabel = modelIssueLabel(model);
            const hasClearableState = modelHasClearableState(model);
            return (
              <tr key={model.internal_id}>
                <td>{model.internal_id}</td>
                <td>
                  <div className="row">
                    <LabeledSwitch
                      compact
                      label="Chat"
                      selected={model.supports_chat}
                      onChange={(value) => updateModel(model, { supports_chat: value })}
                    />
                    <LabeledSwitch
                      compact
                      label="Responses"
                      selected={model.supports_responses}
                      onChange={(value) => updateModel(model, { supports_responses: value })}
                    />
                    <LabeledSwitch
                      compact
                      label="流式"
                      selected={model.supports_stream}
                      onChange={(value) => updateModel(model, { supports_stream: value })}
                    />
                  </div>
                </td>
                <td>{model.context_length || "-"}</td>
                <td>
                  <div className="stack">
                    <LabeledSwitch
                      compact
                      label={model.enabled ? "启用" : "禁用"}
                      selected={model.enabled}
                      onChange={(value) => updateModel(model, { enabled: value })}
                    />
                    {autoDisabledActive ? (
                      <span className="badge badge-danger">自动禁用</span>
                    ) : cooldownText ? (
                      <span className="badge badge-warning">{cooldownText}</span>
                    ) : !autoDisableOn && modelHasAutoState(model) ? (
                      <span className="badge badge-warning">自动禁用已关闭</span>
                    ) : !issueLabel ? (
                      <Status ok text="正常" />
                    ) : null}
                    {issueLabel ? (
                      <span className="badge badge-warning" title={modelIssueTitle(model)}>
                        {issueLabel}
                      </span>
                    ) : null}
                  </div>
                </td>
                <td>
                  {hasClearableState ? (
                    <Button
                      size="sm"
                      variant="secondary"
                      onPress={() =>
                        run(async () => post(`/api/admin/models/${enc(model.internal_id)}/restore`))
                      }
                    >
                      <RotateCcw size={14} />
                      {model.auto_disabled ? "恢复" : "清除"}
                    </Button>
                  ) : null}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

type GroupMemberForm = {
  model_id: string;
  enabled: boolean;
};

type GroupFormState = {
  id: string;
  name: string;
  strategy: ModelGroup["strategy"];
  enabled: boolean;
  members: GroupMemberForm[];
};

type RouteStepForm = {
  target_type: "model" | "group";
  target_id: string;
  enabled: boolean;
};

type RouteFormState = {
  id: string;
  name: string;
  enabled: boolean;
  steps: RouteStepForm[];
};

function blankGroupForm(): GroupFormState {
  return {
    id: "",
    name: "",
    strategy: "fallback",
    enabled: true,
    members: [],
  };
}

function blankRouteForm(): RouteFormState {
  return {
    id: "",
    name: "",
    enabled: true,
    steps: [],
  };
}

function Groups({ data, run }: { data: AppData; run: (task: () => Promise<void>) => void }) {
  const [form, setForm] = useState<GroupFormState>(blankGroupForm);
  const [editingID, setEditingID] = useState("");
  const [open, setOpen] = useState(false);
  const [modelToAdd, setModelToAdd] = useState("");

  const isEditing = Boolean(editingID);
  const selectedModels = new Set(form.members.map((member) => member.model_id));
  const availableModels = data.models.filter((model) => !selectedModels.has(model.internal_id));
  const modelToAddValue = availableModels.some((model) => model.internal_id === modelToAdd)
    ? modelToAdd
    : availableModels[0]?.internal_id ?? "";

  function resetModal() {
    setForm(blankGroupForm());
    setEditingID("");
    setModelToAdd("");
  }

  function openAdd() {
    setForm(blankGroupForm());
    setEditingID("");
    setModelToAdd(data.models[0]?.internal_id ?? "");
    setOpen(true);
  }

  function openEdit(group: ModelGroup) {
    const members = group.members.map((member) => ({
      model_id: member.model_id,
      enabled: member.enabled,
    }));
    const memberIDs = new Set(members.map((member) => member.model_id));
    setForm({
      id: group.id,
      name: group.name,
      strategy: group.strategy,
      enabled: group.enabled,
      members,
    });
    setEditingID(group.id);
    setModelToAdd(data.models.find((model) => !memberIDs.has(model.internal_id))?.internal_id ?? "");
    setOpen(true);
  }

  function addMember() {
    if (!modelToAddValue) return;
    setForm({
      ...form,
      members: [...form.members, { model_id: modelToAddValue, enabled: true }],
    });
    setModelToAdd("");
  }

  function submitGroup(event: FormEvent, close: () => void) {
    event.preventDefault();
    run(async () => {
      const id = form.id.trim();
      const payload: ModelGroup = {
        id,
        name: form.name.trim(),
        strategy: form.strategy,
        enabled: form.enabled,
        members: form.members.map((member, index) => ({
          model_id: member.model_id,
          position: index + 1,
          enabled: member.enabled,
        })),
      };
      if (isEditing) {
        await put(`/api/admin/groups/${enc(editingID)}`, payload);
      } else if (id) {
        await put(`/api/admin/groups/${enc(id)}`, payload);
      } else {
        await post("/api/admin/groups", payload);
      }
      close();
    });
  }

  return (
    <div className="surface">
      <div className="section row list-header">
        <div>
          <h2 style={{ margin: 0 }}>模型组</h2>

        </div>
        <Button onPress={openAdd}>
          <Plus size={16} />
          添加模型组
        </Button>
      </div>

      <ListGroups
        groups={data.groups}
        edit={openEdit}
        remove={(id) => run(async () => del(`/api/admin/groups/${enc(id)}`))}
      />

      <Modal>
        <Modal.Backdrop
          isOpen={open}
          onOpenChange={(nextOpen) => {
            setOpen(nextOpen);
            if (!nextOpen) resetModal();
          }}
          variant="blur"
        >
          <Modal.Container scroll="inside" size="lg">
            <Modal.Dialog className="config-dialog">
              {(dialog: { close: () => void }) => (
                <form className="modal-form" onSubmit={(event) => submitGroup(event, dialog.close)}>
                  <Modal.CloseTrigger />
                  <Modal.Header>
                    <Modal.Heading>{isEditing ? "编辑模型组" : "添加模型组"}</Modal.Heading>

                  </Modal.Header>
                  <Modal.Body className="config-modal-body">
                    <div className="grid-2">
                      <TextField fullWidth isDisabled={isEditing} value={form.id} onChange={(id) => setForm({ ...form, id })}>
                        <Label>组 ID</Label>
                        <Input placeholder="留空自动生成，或填写 gpt-fast" />
                        <Description>保存后不可修改</Description>
                      </TextField>
                      <TextField fullWidth value={form.name} onChange={(name) => setForm({ ...form, name })}>
                        <Label>组名称</Label>
                        <Input placeholder="gpt 快速组" />
                      </TextField>
                    </div>

                    <div className="row">
                      <AppSelect
                        className="select-field"
                        label="策略"
                        value={form.strategy}
                        onChange={(strategy) =>
                          setForm({ ...form, strategy: strategy as ModelGroup["strategy"] })
                        }
                        options={[
                          { id: "fallback", label: "固定顺序" },
                          { id: "random", label: "随机" },
                          { id: "round_robin", label: "轮询" },
                        ]}
                      />
                      <LabeledSwitch
                        label="启用模型组"
                        selected={form.enabled}
                        onChange={(enabled) => setForm({ ...form, enabled })}
                      />
                    </div>

                    <div className="config-editor">
                      <div className="config-add-row">
                        <AppSelect
                          className="select-field select-field-grow"
                          label="添加组内模型"
                          value={modelToAddValue}
                          onChange={setModelToAdd}
                          placeholder={availableModels.length === 0 ? "没有可添加的模型" : "选择模型"}
                          options={availableModels.map((model) => ({
                            id: model.internal_id,
                            label: model.internal_id,
                            textValue: model.internal_id,
                          }))}
                        />
                        <Button
                          type="button"
                          variant="secondary"
                          isDisabled={!modelToAddValue}
                          onPress={addMember}
                        >
                          <Plus size={16} />
                          加入
                        </Button>
                      </div>

                      {form.members.length === 0 ? (
                        <div className="empty-state">暂无组内模型</div>
                      ) : (
                        <div className="config-list">
                          {form.members.map((member, index) => {
                            const model = data.models.find((item) => item.internal_id === member.model_id);
                            return (
                              <div className="config-row" key={member.model_id}>
                                <div className="config-index">{index + 1}</div>
                                <div className="config-row-main">
                                  {member.model_id}
                                  {model?.enabled === false ||
                                  (autoDisableEnabled(data.settings) && model?.auto_disabled) ? (
                                    <Status text="不可用" />
                                  ) : (
                                    <Status ok text="可用" />
                                  )}
                                </div>
                                <div className="row config-row-actions">
                                  <LabeledSwitch
                                    compact
                                    label={member.enabled ? "启用" : "禁用"}
                                    selected={member.enabled}
                                    onChange={(enabled) =>
                                      setForm({
                                        ...form,
                                        members: replaceItem(form.members, index, { ...member, enabled }),
                                      })
                                    }
                                  />
                                  <Button
                                    type="button"
                                    size="sm"
                                    variant="tertiary"
                                    isDisabled={index === 0}
                                    aria-label="上移"
                                    onPress={() => setForm({ ...form, members: moveItem(form.members, index, -1) })}
                                  >
                                    <ArrowUp size={14} />
                                  </Button>
                                  <Button
                                    type="button"
                                    size="sm"
                                    variant="tertiary"
                                    isDisabled={index === form.members.length - 1}
                                    aria-label="下移"
                                    onPress={() => setForm({ ...form, members: moveItem(form.members, index, 1) })}
                                  >
                                    <ArrowDown size={14} />
                                  </Button>
                                  <Button
                                    type="button"
                                    size="sm"
                                    variant="danger"
                                    aria-label="移除"
                                    onPress={() => setForm({ ...form, members: removeItem(form.members, index) })}
                                  >
                                    <Trash2 size={14} />
                                  </Button>
                                </div>
                              </div>
                            );
                          })}
                        </div>
                      )}
                    </div>
                  </Modal.Body>
                  <Modal.Footer>
                    <Button type="button" variant="tertiary" onPress={dialog.close}>
                      取消
                    </Button>
                    <Button type="submit">
                      <Save size={16} />
                      保存模型组
                    </Button>
                  </Modal.Footer>
                </form>
              )}
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </div>
  );
}

function Routes({ data, run }: { data: AppData; run: (task: () => Promise<void>) => void }) {
  const [form, setForm] = useState<RouteFormState>(blankRouteForm);
  const [editingID, setEditingID] = useState("");
  const [open, setOpen] = useState(false);
  const [targetType, setTargetType] = useState<"model" | "group">("group");
  const [targetID, setTargetID] = useState("");

  const isEditing = Boolean(editingID);
  const selectedTargets = new Set(form.steps.map((step) => targetKey(step.target_type, step.target_id)));
  const availableTargets =
    targetType === "group"
      ? data.groups.filter((group) => !selectedTargets.has(targetKey("group", group.id)))
      : data.models.filter((model) => !selectedTargets.has(targetKey("model", model.internal_id)));
  const targetIDValue =
    availableTargets.some((target) => routeTargetID(targetType, target) === targetID)
      ? targetID
      : routeTargetID(targetType, availableTargets[0]) ?? "";

  function resetModal() {
    setForm(blankRouteForm());
    setEditingID("");
    setTargetType(data.groups.length > 0 ? "group" : "model");
    setTargetID("");
  }

  function openAdd() {
    const nextTargetType = data.groups.length > 0 ? "group" : "model";
    setForm(blankRouteForm());
    setEditingID("");
    setTargetType(nextTargetType);
    setTargetID("");
    setOpen(true);
  }

  function openEdit(route: Route) {
    setForm({
      id: route.id,
      name: route.name,
      enabled: route.enabled,
      steps: route.steps.map((step) => ({
        target_type: step.target_type,
        target_id: step.target_id,
        enabled: routeStepEnabled(route, step),
      })),
    });
    setEditingID(route.id);
    setTargetType(data.groups.length > 0 ? "group" : "model");
    setTargetID("");
    setOpen(true);
  }

  function addStep() {
    if (!targetIDValue) return;
    setForm({
      ...form,
      steps: [...form.steps, { target_type: targetType, target_id: targetIDValue, enabled: true }],
    });
    setTargetID("");
  }

  function submitRoute(event: FormEvent, close: () => void) {
    event.preventDefault();
    run(async () => {
      const id = form.id.trim();
      if (!id) {
        throw new Error("请填写虚拟模型 ID");
      }
      const payload: Route = {
        id,
        name: form.name.trim() || id,
        enabled: form.enabled,
        steps: form.steps.map((step, index) => ({
          position: index + 1,
          target_type: step.target_type,
          target_id: step.target_id,
          enabled: step.enabled,
        })),
      };
      const routeID = isEditing ? editingID : id;
      await put(`/api/admin/routes/${enc(routeID)}`, payload);
      if (isEditing) {
        await Promise.all(
          payload.steps.map((step) =>
            post(`/api/admin/routes/${enc(routeID)}/override`, {
              target_type: step.target_type,
              target_id: step.target_id,
              disabled: false,
            }),
          ),
        );
      }
      close();
    });
  }

  return (
    <div className="surface">
      <div className="section row list-header">
        <div>
          <h2 style={{ margin: 0 }}>路由模型</h2>

        </div>
        <Button onPress={openAdd}>
          <Plus size={16} />
          添加路由
        </Button>
      </div>

      <ListRoutes
        routes={data.routes}
        groups={data.groups}
        models={data.models}
        edit={openEdit}
        remove={(id) => run(async () => del(`/api/admin/routes/${enc(id)}`))}
      />

      <Modal>
        <Modal.Backdrop
          isOpen={open}
          onOpenChange={(nextOpen) => {
            setOpen(nextOpen);
            if (!nextOpen) resetModal();
          }}
          variant="blur"
        >
          <Modal.Container scroll="inside" size="lg">
            <Modal.Dialog className="config-dialog">
              {(dialog: { close: () => void }) => (
                <form className="modal-form" onSubmit={(event) => submitRoute(event, dialog.close)}>
                  <Modal.CloseTrigger />
                  <Modal.Header>
                    <Modal.Heading>{isEditing ? "编辑路由" : "添加路由"}</Modal.Heading>

                  </Modal.Header>
                  <Modal.Body className="config-modal-body">
                    <div className="grid-2">
                      <TextField fullWidth isDisabled={isEditing} value={form.id} onChange={(id) => setForm({ ...form, id })}>
                        <Label>虚拟模型 ID</Label>
                        <Input placeholder="coder-fast" />
                      </TextField>
                      <TextField fullWidth value={form.name} onChange={(name) => setForm({ ...form, name })}>
                        <Label>显示名称</Label>
                        <Input placeholder="coder-fast" />
                      </TextField>
                    </div>

                    <LabeledSwitch
                      label="启用路由模型"
                      selected={form.enabled}
                      onChange={(enabled) => setForm({ ...form, enabled })}
                    />

                    <div className="config-editor">
                      <div className="config-add-row route-target-add">
                        <AppSelect
                          className="select-field"
                          label="目标类型"
                          value={targetType}
                          onChange={(type) => {
                            setTargetType(type as "model" | "group");
                            setTargetID("");
                          }}
                          options={[
                            { id: "group", label: "模型组" },
                            { id: "model", label: "单个模型" },
                          ]}
                        />
                        <AppSelect
                          className="select-field select-field-grow"
                          label="添加目标"
                          value={targetIDValue}
                          onChange={setTargetID}
                          placeholder={availableTargets.length === 0 ? "没有可添加的目标" : "选择目标"}
                          options={availableTargets.map((target) => {
                            const id = routeTargetID(targetType, target);
                            const name = routeTargetName(targetType, target);
                            return {
                              id,
                              label: id,
                              textValue: `${name} ${id}`,
                            };
                          })}
                        />
                        <Button
                          type="button"
                          variant="secondary"
                          isDisabled={!targetIDValue}
                          onPress={addStep}
                        >
                          <Plus size={16} />
                          加入
                        </Button>
                      </div>

                      {form.steps.length === 0 ? (
                        <div className="empty-state">暂无目标</div>
                      ) : (
                        <div className="config-list">
                          {form.steps.map((step, index) => (
                            <div className="config-row" key={`${step.target_type}:${step.target_id}`}>
                              <div className="config-index">{index + 1}</div>
                              <div className="config-row-main">
                                <span className="badge">{step.target_type === "group" ? "模型组" : "模型"}</span>
                                {step.target_id}
                              </div>
                              <div className="row config-row-actions">
                                <LabeledSwitch
                                  compact
                                  label={step.enabled ? "启用" : "禁用"}
                                  selected={step.enabled}
                                  onChange={(enabled) =>
                                    setForm({
                                      ...form,
                                      steps: replaceItem(form.steps, index, { ...step, enabled }),
                                    })
                                  }
                                />
                                <Button
                                  type="button"
                                  size="sm"
                                  variant="tertiary"
                                  isDisabled={index === 0}
                                  aria-label="上移"
                                  onPress={() => setForm({ ...form, steps: moveItem(form.steps, index, -1) })}
                                >
                                  <ArrowUp size={14} />
                                </Button>
                                <Button
                                  type="button"
                                  size="sm"
                                  variant="tertiary"
                                  isDisabled={index === form.steps.length - 1}
                                  aria-label="下移"
                                  onPress={() => setForm({ ...form, steps: moveItem(form.steps, index, 1) })}
                                >
                                  <ArrowDown size={14} />
                                </Button>
                                <Button
                                  type="button"
                                  size="sm"
                                  variant="danger"
                                  aria-label="移除"
                                  onPress={() => setForm({ ...form, steps: removeItem(form.steps, index) })}
                                >
                                  <Trash2 size={14} />
                                </Button>
                              </div>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  </Modal.Body>
                  <Modal.Footer>
                    <Button type="button" variant="tertiary" onPress={dialog.close}>
                      取消
                    </Button>
                    <Button type="submit">
                      <Save size={16} />
                      保存路由
                    </Button>
                  </Modal.Footer>
                </form>
              )}
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </div>
  );
}

function RouteSwitch({ data, run }: { data: AppData; run: (task: () => Promise<void>) => void }) {
  const [selectedID, setSelectedID] = useState("");
  const selected = data.routes.find((route) => route.id === (selectedID || data.routes[0]?.id));
  const autoDisableOn = autoDisableEnabled(data.settings);

  function stepDisabled(step: RouteStep) {
    return !routeStepEnabled(selected, step);
  }

  function toggleStep(index: number, enabled: boolean) {
    if (!selected) return;
    const step = selected.steps[index];
    const nextSteps = selected.steps.map((item, currentIndex) =>
      currentIndex === index ? { ...item, enabled } : item,
    );
    run(async () => {
      await put(`/api/admin/routes/${enc(selected.id)}`, {
        id: selected.id,
        name: selected.name,
        enabled: selected.enabled,
        steps: nextSteps,
      });
      await post(`/api/admin/routes/${enc(selected.id)}/override`, {
        target_type: step.target_type,
        target_id: step.target_id,
        disabled: false,
      });
    });
  }

  function modelOverrideDisabled(id: string) {
    return routeOverrideDisabled(selected, "model", id);
  }

  function toggleModelOverride(id: string, enabled: boolean) {
    if (!selected) return;
    run(async () => {
      await post(`/api/admin/routes/${enc(selected.id)}/override`, {
        target_type: "model",
        target_id: id,
        disabled: !enabled,
      });
    });
  }

  const { nodes, edges } = useMemo(() => {
    if (!selected) return { nodes: [] as Node[], edges: [] as Edge[] };
    const top = 72;
    const leftX = 72;
    const routeX = 760;
    const routeHeight = 92;
    const gap = 52;
    let nextY = top;
    const stepLayouts = selected.steps.map((step) => {
      const group =
        step.target_type === "group"
          ? data.groups.find((item) => item.id === step.target_id)
          : undefined;
      const height = step.target_type === "group" ? 94 + (group?.members.length ?? 0) * 36 : 66;
      const layout = { y: nextY, height };
      nextY += height + gap;
      return layout;
    });
    const contentHeight = stepLayouts.length > 0 ? nextY - gap - top : routeHeight;
    const routeY = top + Math.max(0, contentHeight - routeHeight) / 2;
    const nodes: Node[] = [];
    const edges: Edge[] = [];
    nodes.push({
      id: "route",
      position: { x: routeX, y: routeY },
      targetPosition: Position.Left,
      data: {
        label: (
          <FlowBox className="flow-node-route" disabled={!selected.enabled}>
            <div className="flow-node-header">
              <div className="flow-node-heading">
                <span className="flow-node-title" title={selected.name || selected.id}>
                  {selected.name || selected.id}
                </span>
              </div>
              <LabeledSwitch
                compact
                label=""
                selected={selected.enabled}
                onChange={(enabled) =>
                  run(async () => patch(`/api/admin/routes/${enc(selected.id)}/enabled`, { enabled }))
                }
              />
            </div>
            <span className="code" title={selected.id}>
              {selected.id}
            </span>
          </FlowBox>
        ),
      },
      type: "default",
    });
    selected.steps.forEach((step, index) => {
      const y = stepLayouts[index].y;
      if (step.target_type === "group") {
        const group = data.groups.find((item) => item.id === step.target_id);
        const isRouteDisabled = stepDisabled(step);
        const groupState = routeGroupStateLabel(group);
        const isGroupDimmed = isRouteDisabled || Boolean(groupState);
        nodes.push({
          id: `step-${index}`,
          position: { x: leftX, y },
          sourcePosition: Position.Right,
          data: {
            label: (
              <FlowBox className="flow-node-group" disabled={isGroupDimmed}>
                <div className="flow-node-header">
                  <div className="flow-node-heading">
                    <span className="flow-node-title" title={group?.name ?? step.target_id}>
                      {group?.name ?? step.target_id}
                    </span>
                    <span className="flow-node-meta">{group ? strategyLabel(group.strategy) : "模型组"}</span>
                    {groupState ? <span className="flow-node-meta flow-state-label">{groupState}</span> : null}
                  </div>
                  <LabeledSwitch
                    compact
                    label=""
                    selected={!isRouteDisabled}
                    onChange={(enabled) => toggleStep(index, enabled)}
                  />
                </div>
                <div className="flow-member-list">
                  {group?.members.map((member) => {
                    const modelOff = modelOverrideDisabled(member.model_id);
                    const model = member.model ?? data.models.find((item) => item.internal_id === member.model_id);
                    const memberState = routeMemberStateLabel(member, model, data.providers, autoDisableOn);
                    const isMemberDimmed = !isGroupDimmed && (modelOff || Boolean(memberState));
                    return (
                      <div
                        className={`flow-member-row ${isMemberDimmed ? "flow-member-row-dimmed" : ""}`}
                        key={member.model_id}
                      >
                        <div className="flow-node-heading">
                          <span className="code" title={memberState ? `${member.model_id} ${memberState}` : member.model_id}>
                            {member.model_id}
                          </span>
                          {memberState ? <span className="flow-node-meta flow-state-label">{memberState}</span> : null}
                        </div>
                        <LabeledSwitch
                          compact
                          label=""
                          selected={!modelOff}
                          onChange={(enabled) => toggleModelOverride(member.model_id, enabled)}
                        />
                      </div>
                    );
                  })}
                </div>
              </FlowBox>
            ),
          },
          type: "default",
        });
      } else {
        const isRouteDisabled = stepDisabled(step);
        const model = data.models.find((item) => item.internal_id === step.target_id);
        const modelState = routeModelStateLabel(model, data.providers, autoDisableOn);
        const isModelDimmed = isRouteDisabled || Boolean(modelState);
        nodes.push({
          id: `step-${index}`,
          position: { x: leftX, y },
          sourcePosition: Position.Right,
          data: {
            label: (
              <FlowBox className="flow-node-model" disabled={isModelDimmed}>
                <div className="flow-node-header">
                  <div className="flow-node-heading">
                    <span className="code flow-code-main" title={modelState ? `${step.target_id} ${modelState}` : step.target_id}>
                      {step.target_id}
                    </span>
                    {modelState ? <span className="flow-node-meta flow-state-label">{modelState}</span> : null}
                  </div>
                  <LabeledSwitch
                    compact
                    label=""
                    selected={!isRouteDisabled}
                    onChange={(enabled) => toggleStep(index, enabled)}
                  />
                </div>
              </FlowBox>
            ),
          },
          type: "default",
        });
      }
      edges.push({
        id: `edge-${index}`,
        source: `step-${index}`,
        target: "route",
        style: { stroke: "#22863a", strokeWidth: 2 },
        markerEnd: { type: MarkerType.ArrowClosed },
      });
    });
    return { nodes, edges };
  }, [autoDisableOn, data.groups, data.models, data.providers, run, selected]);

  if (!selected) {
    return <div className="surface section muted">暂无路由</div>;
  }

  return (
    <div className="surface">
      <div className="section row">
        <AppSelect
          className="select-field route-select"
          label="代理模型"
          value={selected.id}
          onChange={setSelectedID}
          options={data.routes.map((route) => ({
            id: route.id,
            label: route.name || route.id,
          }))}
        />

      </div>
      <div className="flow-shell">
        <ReactFlow nodes={nodes} edges={edges} fitView fitViewOptions={{ padding: 0.18 }}>
          <Background gap={18} />
          <Controls />
        </ReactFlow>
      </div>
    </div>
  );
}

function LogsView({ logs }: { logs: RequestLog[] }) {
  return (
    <div className="surface table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>时间</th>
            <th>接口</th>
            <th>模型</th>
            <th>结果</th>
            <th>尝试</th>
          </tr>
        </thead>
        <tbody>
          {logs.map((log) => (
            <tr key={log.id}>
              <td>{formatTime(log.created_at)}</td>
              <td>{log.api}</td>
              <td>
                <div className="code">{log.client_model}</div>
                <div className="muted">最终：{log.final_model || "-"}</div>
              </td>
              <td>
                <div className="stack">
                  {log.status === "success" ? (
                    <Status ok text={`${log.http_status} · ${log.duration_ms}ms`} />
                  ) : (
                    <Status text={`${log.http_status} · ${log.duration_ms}ms`} />
                  )}
                  {log.error ? <span className="error">{log.error}</span> : null}
                </div>
              </td>
              <td>
                <div className="stack">
                  {log.attempts?.map((attempt) => (
                    <div key={attempt.id}>
                      <span className="code">{attempt.model_id}</span>{" "}
                      <span className="muted">
                        {attempt.status} · {attempt.http_status} · {attempt.duration_ms}ms
                      </span>
                      {attempt.error ? <div className="error">{attempt.error}</div> : null}
                    </div>
                  ))}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function SettingsView({ data, run }: { data: AppData; run: (task: () => Promise<void>) => void }) {
  const [retention, setRetention] = useState(data.settings.log_retention_days ?? "30");
  const [upstreamTimeout, setUpstreamTimeout] = useState(data.settings.upstream_timeout_seconds ?? "20");
  const [keyName, setKeyName] = useState("默认客户端");
  const [copiedKey, setCopiedKey] = useState("");
  const [visibleTokens, setVisibleTokens] = useState<Record<string, string>>({});
  const [copyError, setCopyError] = useState("");
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");

  async function getToken(id: string) {
    if (visibleTokens[id]) {
      return visibleTokens[id];
    }
    const { token } = await api<{ token: string }>(`/api/admin/proxy-keys/${enc(id)}/token`);
    return token;
  }

  async function toggleToken(id: string) {
    setCopyError("");
    if (visibleTokens[id]) {
      setVisibleTokens((current) => {
        const next = { ...current };
        delete next[id];
        return next;
      });
      return;
    }
    try {
      const token = await getToken(id);
      setVisibleTokens((current) => ({ ...current, [id]: token }));
    } catch (err) {
      setCopyError(err instanceof Error ? err.message : "显示失败，请稍后重试。");
    }
  }

  async function copyToken(id: string) {
    setCopyError("");
    try {
      const token = await getToken(id);
      await copyText(token);
      setCopiedKey(id);
      window.setTimeout(() => {
        setCopiedKey((current) => (current === id ? "" : current));
      }, 1600);
    } catch (err) {
      setCopyError(err instanceof Error ? err.message : "复制失败，请稍后重试。");
    }
  }

  return (
    <div className="surface">
      <div className="section stack">
        <h2>故障切换</h2>
        <LabeledSwitch
          label="自动禁用失败模型"
          selected={autoDisableEnabled(data.settings)}
          onChange={(selected) =>
            run(async () => {
              await put("/api/admin/settings", { auto_disable_models: selected ? "true" : "false" });
            })
          }
        />
        <div className="row">
          <TextField value={upstreamTimeout} onChange={setUpstreamTimeout}>
            <Label>单模型超时秒数</Label>
            <Input />
          </TextField>
          <Button
            variant="secondary"
            onPress={() =>
              run(async () => {
                await put("/api/admin/settings", { upstream_timeout_seconds: upstreamTimeout });
              })
            }
          >
            <Save size={16} />
            保存
          </Button>
        </div>
      </div>

      <div className="section stack">
        <h2>/v1/models</h2>
        <LabeledSwitch
          label="暴露原始模型"
          selected={data.settings.models_expose_raw === "true"}
          onChange={(selected) =>
            run(async () => {
              await put("/api/admin/settings", { models_expose_raw: selected ? "true" : "false" });
            })
          }
        />
        <div className="row">
          <TextField value={retention} onChange={setRetention}>
            <Label>日志保留天数</Label>
            <Input />
          </TextField>
          <Button
            variant="secondary"
            onPress={() =>
              run(async () => {
                await put("/api/admin/settings", { log_retention_days: retention });
              })
            }
          >
            <Save size={16} />
            保存
          </Button>
        </div>
      </div>

      <div className="section stack">
        <h2>代理访问密钥</h2>
        {copyError ? <div className="error">{copyError}</div> : null}
        <div className="row key-create-row">
          <TextField value={keyName} onChange={setKeyName}>
            <Label>密钥名称</Label>
            <Input />
          </TextField>
          <Button
            onPress={() =>
              run(async () => {
                await post<ProxyKey>("/api/admin/proxy-keys", { name: keyName });
              })
            }
          >
            <KeyRound size={16} />
            创建密钥
          </Button>
        </div>
        <table className="data-table">
          <thead>
            <tr>
              <th>名称</th>
              <th>密钥</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {data.keys.map((key) => {
              const copied = copiedKey === key.id;
              const visibleToken = visibleTokens[key.id];
              return (
                <tr key={key.id}>
                  <td>{key.name}</td>
                  <td>
                    <div className="key-token">
                      <span className="code secret-code">{visibleToken || `${key.prefix}...`}</span>
                      <div className="row key-actions">
                        <Button size="sm" variant="secondary" onPress={() => toggleToken(key.id)}>
                          {visibleToken ? <EyeOff size={14} /> : <Eye size={14} />}
                          {visibleToken ? "隐藏" : "显示"}
                        </Button>
                        <Button size="sm" variant="secondary" onPress={() => copyToken(key.id)}>
                          {copied ? <Check size={14} /> : <Copy size={14} />}
                          {copied ? "已复制" : "复制"}
                        </Button>
                      </div>
                    </div>
                  </td>
                  <td>
                    <LabeledSwitch
                      compact
                      label={key.enabled ? "启用" : "禁用"}
                      selected={key.enabled}
                      onChange={(enabled) =>
                        run(async () => patch(`/api/admin/proxy-keys/${enc(key.id)}`, { enabled }))
                      }
                    />
                  </td>
                  <td>
                    <Button
                      size="sm"
                      variant="danger"
                      onPress={() => run(async () => del(`/api/admin/proxy-keys/${enc(key.id)}`))}
                    >
                      <Trash2 size={14} />
                    </Button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <form
        className="section stack"
        onSubmit={(event) => {
          event.preventDefault();
          run(async () => {
            await post("/api/admin/change-password", {
              old_password: oldPassword,
              new_password: newPassword,
            });
            setOldPassword("");
            setNewPassword("");
          });
        }}
      >
        <h2>管理员密码</h2>
        <div className="grid-2">
          <TextField type="password" value={oldPassword} onChange={setOldPassword}>
            <Label>旧密码</Label>
            <Input />
          </TextField>
          <TextField type="password" value={newPassword} onChange={setNewPassword}>
            <Label>新密码</Label>
            <Input />
          </TextField>
        </div>
        <Button type="submit" variant="secondary">
          <Save size={16} />
          修改密码
        </Button>
      </form>
    </div>
  );
}

function ListGroups({
  groups,
  edit,
  remove,
}: {
  groups: ModelGroup[];
  edit: (group: ModelGroup) => void;
  remove: (id: string) => void;
}) {
  return (
    <div className="section stack">
      {groups.length === 0 ? (
        <div className="empty-state">暂无模型组</div>
      ) : null}
      {groups.map((group) => (
        <div className="row" key={group.id} style={{ justifyContent: "space-between" }}>
          <div>
            <strong>{group.name}</strong> <span className="code">{group.id}</span>
            <div className="muted">
              {strategyLabel(group.strategy)} · {group.members.length} 个模型
            </div>
            {group.members.length > 0 ? (
              <div className="muted list-preview">
                {group.members.slice(0, 3).map((member) => member.model_id).join(" → ")}
                {group.members.length > 3 ? " ..." : ""}
              </div>
            ) : null}
          </div>
          <div className="row">
            <Button size="sm" variant="tertiary" onPress={() => edit(group)}>
              编辑
            </Button>
            <Button size="sm" variant="danger" onPress={() => remove(group.id)}>
              <Trash2 size={14} />
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}

function ListRoutes({
  routes,
  groups,
  models,
  edit,
  remove,
}: {
  routes: Route[];
  groups: ModelGroup[];
  models: Model[];
  edit: (route: Route) => void;
  remove: (id: string) => void;
}) {
  return (
    <div className="section stack">
      {routes.length === 0 ? (
        <div className="empty-state">暂无路由</div>
      ) : null}
      {routes.map((route) => {
        const closedCount = routeClosedCount(route);
        const previewSteps = route.steps.slice(0, 3);
        return (
          <div className="row" key={route.id} style={{ justifyContent: "space-between" }}>
            <div>
              <strong>{route.name}</strong> <span className="code">{route.id}</span>
              <div className="muted">
                {route.steps.length} 个目标 · {route.enabled ? "启用" : "禁用"} ·{" "}
                {closedCount > 0 ? `${closedCount} 项关闭` : "全部打开"}
              </div>
              {route.steps.length > 0 ? (
                <div className="muted list-preview">
                  {previewSteps.map((step, index) => {
                    const name =
                      step.target_type === "group"
                        ? groupName(groups, step.target_id)
                        : modelName(models, step.target_id);
                    const enabled = routeStepEnabled(route, step);
                    return (
                      <span className={enabled ? undefined : "route-target-off"} key={`${step.target_type}:${step.target_id}`}>
                        {name}
                        {enabled ? "" : "（关闭）"}
                        {index < previewSteps.length - 1 ? " → " : ""}
                      </span>
                    );
                  })}
                  {route.steps.length > 3 ? " ..." : ""}
                </div>
              ) : null}
            </div>
            <div className="row">
              <Button size="sm" variant="tertiary" onPress={() => edit(route)}>
                编辑
              </Button>
              <Button size="sm" variant="danger" onPress={() => remove(route.id)}>
                <Trash2 size={14} />
              </Button>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function strategyLabel(strategy: ModelGroup["strategy"]) {
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

function modelName(models: Model[], id: string) {
  return id;
}

function groupName(groups: ModelGroup[], id: string) {
  const group = groups.find((item) => item.id === id);
  return group?.name || id;
}

function routeModelStateLabel(model: Model | undefined, providers: Provider[], autoDisableOn: boolean) {
  if (!model) return "";
  if (!routeModelProviderEnabled(model, providers)) return "提供商未启用";
  if (!model.enabled) return "未启用";
  if (autoDisableOn && model.auto_disabled) return "已禁用";
  if (autoDisableOn && modelCoolingDown(model)) return "冷却中";
  return "";
}

function routeMemberStateLabel(
  member: ModelGroupMember,
  model: Model | undefined,
  providers: Provider[],
  autoDisableOn: boolean,
) {
  if (!member.enabled) return "未启用";
  return routeModelStateLabel(model, providers, autoDisableOn);
}

function routeModelProviderEnabled(model: Model, providers: Provider[]) {
  const provider = providers.find((item) => item.id === model.provider_id);
  if (provider) return provider.enabled;
  return model.provider_enabled !== false;
}

function routeGroupStateLabel(group: ModelGroup | undefined) {
  if (!group) return "";
  return group.enabled ? "" : "未启用";
}

function targetKey(type: "model" | "group", id: string) {
  return `${type}:${id}`;
}

function routeOverrideDisabled(route: Route | undefined, type: "model" | "group", id: string) {
  return route?.overrides?.some((item) => item.target_type === type && item.target_id === id && item.disabled) ?? false;
}

function routeStepEnabled(route: Route | undefined, step: Pick<RouteStep, "target_type" | "target_id" | "enabled">) {
  return step.enabled && !routeOverrideDisabled(route, step.target_type, step.target_id);
}

function routeClosedCount(route: Route) {
  const closedTargets = new Set(
    route.steps.filter((step) => !routeStepEnabled(route, step)).map((step) => targetKey(step.target_type, step.target_id)),
  );
  const overrideOnly =
    route.overrides?.filter((item) => item.disabled && !closedTargets.has(targetKey(item.target_type, item.target_id)))
      .length ?? 0;
  return closedTargets.size + overrideOnly;
}

function routeTargetID(type: "model" | "group", target?: ModelGroup | Model) {
  if (!target) return "";
  return type === "group" ? (target as ModelGroup).id : (target as Model).internal_id;
}

function routeTargetName(type: "model" | "group", target: ModelGroup | Model) {
  return type === "group"
    ? (target as ModelGroup).name || (target as ModelGroup).id
    : (target as Model).internal_id;
}

function routeStepName(data: AppData, step: RouteStepForm) {
  return step.target_type === "group"
    ? groupName(data.groups, step.target_id)
    : modelName(data.models, step.target_id);
}

function autoDisableEnabled(settings: Settings) {
  return settings.auto_disable_models !== "false";
}

const modelCooldownLimit = 3;

function modelCooldownLabel(model: Model) {
  const count = model.cooldown_count ?? 0;
  if (count <= 0) return "";
  const label = `冷却 ${Math.min(count, modelCooldownLimit)}/${modelCooldownLimit}`;
  if (!modelCoolingDown(model)) return label;
  return `${label} · ${modelCooldownRemaining(model)}`;
}

function modelCoolingDown(model: Model) {
  const until = Date.parse(model.cooldown_until || "");
  return Number.isFinite(until) && until > Date.now();
}

function modelCooldownRemaining(model: Model) {
  const until = Date.parse(model.cooldown_until || "");
  const minutes = Math.max(1, Math.ceil((until - Date.now()) / 60000));
  return `${minutes}m`;
}

function modelHasAutoState(model: Model) {
  return model.auto_disabled || (model.cooldown_count ?? 0) > 0 || Boolean(model.cooldown_until);
}

function modelHasClearableState(model: Model) {
  return modelHasAutoState(model) || (model.upstream_error_status ?? 0) > 0;
}

function modelIssueLabel(model: Model) {
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

function modelIssueTitle(model: Model) {
  const parts = [
    model.upstream_error_at ? `时间：${formatTime(model.upstream_error_at)}` : "",
    model.upstream_error,
  ].filter(Boolean);
  return parts.join("\n");
}

function replaceItem<T>(items: T[], index: number, item: T) {
  return items.map((current, currentIndex) => (currentIndex === index ? item : current));
}

function removeItem<T>(items: T[], index: number) {
  return items.filter((_, currentIndex) => currentIndex !== index);
}

function moveItem<T>(items: T[], index: number, delta: number) {
  const nextIndex = index + delta;
  if (nextIndex < 0 || nextIndex >= items.length) return items;
  const next = [...items];
  const [item] = next.splice(index, 1);
  next.splice(nextIndex, 0, item);
  return next;
}

function AppSelect({
  label,
  value,
  onChange,
  options,
  placeholder = "请选择",
  className,
  isDisabled,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  className?: string;
  isDisabled?: boolean;
}) {
  return (
    <Select
      className={className}
      fullWidth
      isDisabled={isDisabled || options.length === 0}
      placeholder={placeholder}
      value={value || null}
      variant="secondary"
      onChange={(nextValue) => onChange(nextValue === null ? "" : String(nextValue))}
    >
      <Label>{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="app-select-popover">
        <ListBox>
          {options.map((option) => (
            <ListBox.Item id={option.id} key={option.id} textValue={option.textValue ?? String(option.label)}>
              <span className="select-option-label">{option.label}</span>
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  );
}

function LabeledSwitch({
  label,
  selected,
  onChange,
  compact,
}: {
  label: string;
  selected: boolean;
  onChange: (selected: boolean) => void;
  compact?: boolean;
}) {
  return (
    <Switch isSelected={selected} size={compact ? "sm" : "md"} onChange={onChange}>
      <Switch.Control>
        <Switch.Thumb />
      </Switch.Control>
      {label ? (
        <Switch.Content>
          <Label className={compact ? "text-xs" : "text-sm"}>{label}</Label>
        </Switch.Content>
      ) : null}
    </Switch>
  );
}

function Status({ ok, text }: { ok?: boolean; text: string }) {
  return <span className={`badge ${ok ? "badge-success" : "badge-danger"}`}>{text}</span>;
}

function FlowBox({
  children,
  className,
  disabled,
}: {
  children: ReactNode;
  className?: string;
  disabled?: boolean;
}) {
  return <div className={`flow-node ${className ?? ""} ${disabled ? "flow-node-disabled" : ""}`}>{children}</div>;
}

function formatTime(value: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

async function copyText(text: string) {
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
