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
  ProviderKey,
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
  { key: "switch", label: "璺敱寮€鍏?, icon: <Cable size={17} /> },
  { key: "providers", label: "鎻愪緵鍟嗛厤缃?, icon: <Boxes size={17} /> },
  { key: "models", label: "妯″瀷鍒楄〃", icon: <ListTree size={17} /> },
  { key: "groups", label: "妯″瀷缁?, icon: <GitBranch size={17} /> },
  { key: "routes", label: "璺敱閰嶇疆", icon: <Activity size={17} /> },
  { key: "logs", label: "鏃ュ織", icon: <Logs size={17} /> },
  { key: "settings", label: "璁剧疆", icon: <SettingsIcon size={17} /> },
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
    return <div className="login-page muted">姝ｅ湪鍔犺浇...</div>;
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
          榛樿鍦板潃锛?27.0.0.1:2778
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
              鍒锋柊
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
              閫€鍑?            </Button>
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
          <h1 className="page-title">鐧诲綍 Easy Router</h1>
          <div className="page-subtitle">棣栨鍚姩瀵嗙爜浼氭樉绀哄湪鍚庣鎺у埗鍙般€?/div>
        </div>
        {error ? <div className="error">{error}</div> : null}
        <TextField fullWidth value={username} onChange={setUsername}>
          <Label>鐢ㄦ埛鍚?/Label>
          <Input placeholder="admin" />
        </TextField>
        <TextField fullWidth type="password" value={password} onChange={setPassword}>
          <Label>瀵嗙爜</Label>
          <Input placeholder="杈撳叆绠＄悊鍛樺瘑鐮? />
        </TextField>
        <Button type="submit">
          <Shield size={16} />
          鐧诲綍
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
    multi_key_enabled: false,
    multi_key_strategy: "round_robin",
  };
  const [form, setForm] = useState<Provider>(blankProvider);
  const [headersText, setHeadersText] = useState("{}");
  const [editing, setEditing] = useState(false);
  const [providerKeys, setProviderKeys] = useState<ProviderKey[]>([]);
  const [pendingKeys, setPendingKeys] = useState<ProviderKey[]>([]);
  const [keyNames, setKeyNames] = useState<Record<string, string>>({});
  const [newKeyName, setNewKeyName] = useState("");
  const [newKeyValue, setNewKeyValue] = useState("");
  const [rotateValues, setRotateValues] = useState<Record<string, string>>({});
  const [keyError, setKeyError] = useState("");
  const [expanded, setExpanded] = useState("");
  const [remoteModels, setRemoteModels] = useState<Record<string, RemoteModel[]>>({});
  const [selectedRemote, setSelectedRemote] = useState<Record<string, string[]>>({});
  const [manualModel, setManualModel] = useState<Record<string, string>>({});
  const [deletingProvider, setDeletingProvider] = useState<Provider | null>(null);
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
  const deletingProviderModelCount = deletingProvider
    ? (modelsByProvider.get(deletingProvider.id)?.length ?? 0)
    : 0;
  const existingProvider = data.providers.some((provider) => provider.id === form.id);

  async function loadProviderKeys(providerID: string) {
    if (!providerID) {
      setProviderKeys([]);
      setKeyNames({});
      return;
    }
    setKeyError("");
    try {
      const keys = await api<ProviderKey[]>(`/api/admin/providers/${enc(providerID)}/keys`);
      setProviderKeys(keys);
      setKeyNames(Object.fromEntries(keys.map((key) => [key.id, key.name])));
    } catch (err) {
      setKeyError(err instanceof Error ? err.message : "Key 鍒楄〃鍔犺浇澶辫触");
    }
  }

  function openAdd() {
    setForm(blankProvider);
    setHeadersText("{}");
    setProviderKeys([]);
    setPendingKeys([]);
    setKeyNames({});
    setNewKeyName("");
    setNewKeyValue("");
    setRotateValues({});
    setKeyError("");
    setEditing(true);
  }

  function openEdit(provider: Provider) {
    setForm({ ...provider, api_key: "" });
    setHeadersText(JSON.stringify(provider.extra_headers ?? {}, null, 2));
    setPendingKeys([]);
    setNewKeyName("");
    setNewKeyValue("");
    setRotateValues({});
    setKeyError("");
    void loadProviderKeys(provider.id);
    setEditing(true);
  }

  function closeEditor() {
    setEditing(false);
    setForm(blankProvider);
    setHeadersText("{}");
    setProviderKeys([]);
    setPendingKeys([]);
    setKeyNames({});
    setNewKeyName("");
    setNewKeyValue("");
    setRotateValues({});
    setKeyError("");
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    run(async () => {
      const headers = headersText.trim() ? JSON.parse(headersText) : {};
      const providerID = form.id.trim();
      const payload = { ...form, id: providerID, extra_headers: headers };
      if (data.providers.some((provider) => provider.id === providerID)) {
        await put(`/api/admin/providers/${enc(providerID)}`, payload);
      } else {
        await post("/api/admin/providers", payload);
      }
      if (payload.multi_key_enabled) {
        for (const key of pendingKeys) {
          await post(`/api/admin/providers/${enc(providerID)}/keys`, {
            name: key.name,
            api_key: key.api_key,
          });
        }
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

  function addPendingKey() {
    const value = newKeyValue.trim();
    if (!value) {
      setKeyError("璇峰厛濉啓 API Key");
      return;
    }
    const nextIndex = pendingKeys.length + 1;
    const key: ProviderKey = {
      id: `pending-${Date.now()}`,
      provider_id: form.id,
      name: newKeyName.trim() || `Key ${nextIndex}`,
      api_key: value,
      prefix: value.length <= 8 ? value : value.slice(0, 8),
      enabled: true,
      position: nextIndex,
      model_issue_count: 0,
      created_at: "",
      updated_at: "",
    };
    setPendingKeys([...pendingKeys, key]);
    setNewKeyName("");
    setNewKeyValue("");
    setKeyError("");
  }

  function movePendingKey(index: number, direction: number) {
    setPendingKeys(moveItem(pendingKeys, index, direction).map((key, itemIndex) => ({
      ...key,
      position: itemIndex + 1,
    })));
  }

  function updatePendingKey(index: number, patchData: Partial<ProviderKey>) {
    setPendingKeys(replaceItem(pendingKeys, index, { ...pendingKeys[index], ...patchData }));
  }

  function removePendingKey(index: number) {
    setPendingKeys(removeItem(pendingKeys, index).map((key, itemIndex) => ({
      ...key,
      position: itemIndex + 1,
    })));
  }

  function runKeyTask(task: () => Promise<void>) {
    run(async () => {
      await task();
      if (form.id) {
        await loadProviderKeys(form.id);
      }
    });
  }

  function confirmDeleteProvider() {
    if (!deletingProvider) return;
    const providerID = deletingProvider.id;
    setDeletingProvider(null);
    run(async () => del(`/api/admin/providers/${enc(providerID)}`));
  }

  return (
    <div className="surface">
      <div className="section row" style={{ justifyContent: "space-between" }}>
        <div>
          <h2 style={{ margin: 0 }}>鎻愪緵鍟?/h2>

        </div>
        <Button onPress={openAdd}>
          <Plus size={16} />
          娣诲姞鎻愪緵鍟?        </Button>
      </div>

      <div className="provider-list">
        {data.providers.length === 0 ? (
          <div className="section muted">鏆傛棤鎻愪緵鍟?/div>
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
                  title={isOpen ? "鏀惰捣" : "灞曞紑"}
                >
                  {isOpen ? <ChevronDown size={18} /> : <ChevronRight size={18} />}
                </button>
                <div className="provider-title">
                  <strong>{provider.name || provider.id}</strong>
                  <span className="code">{provider.id}</span>
                  <span className="muted">{provider.base_url}</span>
                </div>
                <div className="row provider-actions">
                  {provider.enabled ? <Status ok text="鍚敤" /> : <Status text="绂佺敤" />}
                  {provider.multi_key_enabled ? (
                    <span className="badge">
                      澶?Key {provider.enabled_key_count ?? 0}/{provider.key_count ?? 0} 路 {strategyLabel(provider.multi_key_strategy)}
                    </span>
                  ) : null}
                  <Button size="sm" variant="secondary" onPress={() => openEdit(provider)}>
                    <Edit3 size={14} />
                    缂栬緫
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    onPress={() => setDeletingProvider(provider)}
                  >
                    <Trash2 size={14} />
                  </Button>
                </div>
              </div>

              {isOpen ? (
                <div className="provider-detail">
                  <div className="row" style={{ justifyContent: "space-between" }}>
                    <h3 style={{ margin: 0 }}>宸叉坊鍔犳ā鍨?/h3>
                    <div className="row">
                      <input
                        value={manualModel[provider.id] ?? ""}
                        onChange={(event) =>
                          setManualModel({ ...manualModel, [provider.id]: event.target.value })
                        }
                        placeholder="鎵嬪姩杈撳叆鍘熸ā鍨?ID"
                      />
                      <Button
                        size="sm"
                        variant="secondary"
                        onPress={() =>
                          run(async () => {
                            const originalID = (manualModel[provider.id] ?? "").trim();
                            if (!originalID) {
                              throw new Error("璇峰厛杈撳叆妯″瀷 ID");
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
                        鎵嬪姩娣诲姞
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
                        鎷夊彇妯″瀷
                      </Button>
                    </div>
                  </div>

                  <ProviderModels models={providerModels} autoDisableEnabled={autoDisableOn} />

                  {discovered.length > 0 ? (
                    <div className="model-pick-list">
                      <div className="row" style={{ justifyContent: "space-between" }}>
                        <strong>鎷夊彇缁撴灉</strong>
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
                          瀵煎叆閫変腑
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
                            {model.already_imported ? <span className="badge">宸叉坊鍔?/span> : null}
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
            <Modal.Dialog
              className={`config-dialog provider-config-dialog ${
                form.multi_key_enabled ? "provider-config-dialog-multi" : ""
              }`}
            >
              <form className="modal-form" onSubmit={submit}>
                <Modal.CloseTrigger />
                <Modal.Header>
                  <Modal.Heading>
                    {existingProvider ? "缂栬緫鎻愪緵鍟? : "娣诲姞鎻愪緵鍟?}
                  </Modal.Heading>

                </Modal.Header>
                <Modal.Body className="config-modal-body provider-editor-body">
                  <div
                    className={`provider-editor-grid ${
                      form.multi_key_enabled ? "provider-editor-grid-multi" : ""
                    }`}
                  >
                    <div className="provider-editor-main">
                      <div className="grid-2">
                        <TextField
                          fullWidth
                          isDisabled={existingProvider}
                          value={form.id}
                          onChange={(id) => setForm({ ...form, id })}
                        >
                          <Label>鎻愪緵鍟?ID</Label>
                          <Input placeholder="openai" />
                          <Description>淇濆瓨鍚庝笉鍙慨鏀?/Description>
                        </TextField>
                        <TextField fullWidth value={form.name} onChange={(name) => setForm({ ...form, name })}>
                          <Label>鏄剧ず鍚嶇О</Label>
                          <Input placeholder="OpenAI" />
                        </TextField>
                      </div>
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
                        isDisabled={form.multi_key_enabled}
                        value={form.multi_key_enabled ? "" : (form.api_key ?? "")}
                        onChange={(api_key) => setForm({ ...form, api_key })}
                      >
                        <Label>API Key</Label>
                        <Input placeholder={form.multi_key_enabled ? "鐢卞彸渚у Key 绠＄悊" : "缂栬緫鏃剁暀绌鸿〃绀轰笉淇敼"} />
                      </TextField>
                      <TextField fullWidth value={headersText} onChange={setHeadersText}>
                        <Label>棰濆璇锋眰澶?JSON</Label>
                        <TextArea rows={3} placeholder='{"HTTP-Referer":"http://localhost"}' />
                      </TextField>
                      <div className="provider-switches">
                        <LabeledSwitch
                          label="鍚敤鎻愪緵鍟?
                          selected={form.enabled}
                          onChange={(enabled) => setForm({ ...form, enabled })}
                        />
                        <LabeledSwitch
                          label="鍚敤澶?Key"
                          selected={form.multi_key_enabled}
                          onChange={(multi_key_enabled) =>
                            setForm({
                              ...form,
                              multi_key_enabled,
                              multi_key_strategy: form.multi_key_strategy || "round_robin",
                            })
                          }
                        />
                      </div>
                    </div>
                    {form.multi_key_enabled ? (
                      <ProviderKeyPanel
                        existingProvider={existingProvider}
                        keys={existingProvider ? providerKeys : pendingKeys}
                        keyNames={keyNames}
                        keyError={keyError}
                        newKeyName={newKeyName}
                        newKeyValue={newKeyValue}
                        rotateValues={rotateValues}
                        strategy={form.multi_key_strategy || "round_robin"}
                        onStrategyChange={(multi_key_strategy) => setForm({ ...form, multi_key_strategy })}
                        onNewKeyNameChange={setNewKeyName}
                        onNewKeyValueChange={setNewKeyValue}
                        onAddKey={() => {
                          if (existingProvider) {
                            runKeyTask(async () => {
                              await post(`/api/admin/providers/${enc(form.id)}/keys`, {
                                name: newKeyName,
                                api_key: newKeyValue,
                              });
                              setNewKeyName("");
                              setNewKeyValue("");
                            });
                          } else {
                            addPendingKey();
                          }
                        }}
                        onNameDraftChange={(id, name) => setKeyNames({ ...keyNames, [id]: name })}
                        onSaveName={(key) =>
                          runKeyTask(async () => {
                            await patch(`/api/admin/providers/${enc(form.id)}/keys/${enc(key.id)}`, {
                              id: key.id,
                              name: keyNames[key.id] ?? key.name,
                              enabled: key.enabled,
                            });
                          })
                        }
                        onToggleKey={(key, enabled) =>
                          runKeyTask(async () => {
                            await patch(`/api/admin/providers/${enc(form.id)}/keys/${enc(key.id)}`, {
                              id: key.id,
                              name: keyNames[key.id] ?? key.name,
                              enabled,
                            });
                          })
                        }
                        onRotateValueChange={(id, value) => setRotateValues({ ...rotateValues, [id]: value })}
                        onRotate={(key) =>
                          runKeyTask(async () => {
                            await post(`/api/admin/providers/${enc(form.id)}/keys/${enc(key.id)}/rotate`, {
                              api_key: rotateValues[key.id] ?? "",
                            });
                            setRotateValues({ ...rotateValues, [key.id]: "" });
                          })
                        }
                        onRestore={(key) =>
                          runKeyTask(async () => {
                            await post(`/api/admin/providers/${enc(form.id)}/keys/${enc(key.id)}/restore`);
                          })
                        }
                        onDelete={(key) =>
                          runKeyTask(async () => {
                            await del(`/api/admin/providers/${enc(form.id)}/keys/${enc(key.id)}`);
                          })
                        }
                        onMove={(index, direction) => {
                          if (existingProvider) {
                            const nextKeys = moveItem(providerKeys, index, direction);
                            setProviderKeys(nextKeys);
                            runKeyTask(async () => {
                              await put(`/api/admin/providers/${enc(form.id)}/keys/order`, {
                                ids: nextKeys.map((key) => key.id),
                              });
                            });
                          } else {
                            movePendingKey(index, direction);
                          }
                        }}
                        onPendingChange={updatePendingKey}
                        onPendingDelete={removePendingKey}
                      />
                    ) : null}
                  </div>
                </Modal.Body>
                <Modal.Footer>
                  <Button type="button" variant="tertiary" onPress={closeEditor}>
                    鍙栨秷
                  </Button>
                  <Button type="submit">
                    <Save size={16} />
                    淇濆瓨
                  </Button>
                </Modal.Footer>
              </form>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
      <DeleteConfirmModal
        isOpen={Boolean(deletingProvider)}
        title="鍒犻櫎鎻愪緵鍟?
        targetName={deletingProvider ? deletingProvider.name || deletingProvider.id : ""}
        description={providerDeleteDescription(deletingProviderModelCount)}
        onCancel={() => setDeletingProvider(null)}
        onConfirm={confirmDeleteProvider}
      />
    </div>
  );
}

function ProviderKeyPanel({
  existingProvider,
  keys,
  keyNames,
  keyError,
  newKeyName,
  newKeyValue,
  rotateValues,
  strategy,
  onStrategyChange,
  onNewKeyNameChange,
  onNewKeyValueChange,
  onAddKey,
  onNameDraftChange,
  onSaveName,
  onToggleKey,
  onRotateValueChange,
  onRotate,
  onRestore,
  onDelete,
  onMove,
  onPendingChange,
  onPendingDelete,
}: {
  existingProvider: boolean;
  keys: ProviderKey[];
  keyNames: Record<string, string>;
  keyError: string;
  newKeyName: string;
  newKeyValue: string;
  rotateValues: Record<string, string>;
  strategy: Provider["multi_key_strategy"];
  onStrategyChange: (strategy: Provider["multi_key_strategy"]) => void;
  onNewKeyNameChange: (value: string) => void;
  onNewKeyValueChange: (value: string) => void;
  onAddKey: () => void;
  onNameDraftChange: (id: string, name: string) => void;
  onSaveName: (key: ProviderKey) => void;
  onToggleKey: (key: ProviderKey, enabled: boolean) => void;
  onRotateValueChange: (id: string, value: string) => void;
  onRotate: (key: ProviderKey) => void;
  onRestore: (key: ProviderKey) => void;
  onDelete: (key: ProviderKey) => void;
  onMove: (index: number, direction: number) => void;
  onPendingChange: (index: number, patchData: Partial<ProviderKey>) => void;
  onPendingDelete: (index: number) => void;
}) {
  return (
    <div className="provider-key-panel">
      <div className="provider-key-heading">
        <div>
          <h3>澶?Key 绠＄悊</h3>
          <div className="muted">{existingProvider ? `${keys.length} 涓?Key` : "淇濆瓨鏃跺垱寤鸿繖浜?Key"}</div>
        </div>
        <AppSelect
          className="select-field provider-key-strategy"
          label="璇锋眰绛栫暐"
          value={strategy}
          onChange={(value) => onStrategyChange(value as Provider["multi_key_strategy"])}
          options={[
            { id: "round_robin", label: "杞" },
            { id: "fallback", label: "鍥哄畾椤哄簭" },
            { id: "random", label: "闅忔満" },
          ]}
        />
      </div>

      {keyError ? <div className="error">{keyError}</div> : null}

      <div className="provider-key-add">
        <TextField value={newKeyName} onChange={onNewKeyNameChange}>
          <Label>Key 鍚嶇О</Label>
          <Input placeholder={`Key ${keys.length + 1}`} />
        </TextField>
        <TextField type="password" value={newKeyValue} onChange={onNewKeyValueChange}>
          <Label>API Key</Label>
          <Input placeholder="sk-..." />
        </TextField>
        <Button type="button" variant="secondary" onPress={onAddKey}>
          <Plus size={16} />
          娣诲姞
        </Button>
      </div>

      {keys.length === 0 ? (
        <div className="empty-state">鏆傛棤 Key</div>
      ) : (
        <div className="provider-key-list">
          {keys.map((key, index) => {
            const draftName = existingProvider ? (keyNames[key.id] ?? key.name) : key.name;
            return (
              <div className="provider-key-row" key={key.id}>
                <div className="provider-key-index">{index + 1}</div>
                <div className="provider-key-main">
                  <input
                    value={draftName}
                    onChange={(event) =>
                      existingProvider
                        ? onNameDraftChange(key.id, event.target.value)
                        : onPendingChange(index, { name: event.target.value })
                    }
                  />
                  <div className="row">
                    <span className="code">{key.prefix ? `${key.prefix}...` : "寰呬繚瀛?}</span>
                    {key.model_issue_count > 0 ? (
                      <span className="badge badge-warning">{key.model_issue_count} 涓紓甯告ā鍨?/span>
                    ) : (
                      <span className="badge badge-success">鐘舵€佹甯?/span>
                    )}
                  </div>
                  {existingProvider ? (
                    <div className="provider-key-rotate">
                      <input
                        type="password"
                        value={rotateValues[key.id] ?? ""}
                        onChange={(event) => onRotateValueChange(key.id, event.target.value)}
                        placeholder="濉啓鏂?API Key 鍚庤疆鎹?
                      />
                      <Button type="button" size="sm" variant="secondary" onPress={() => onRotate(key)}>
                        <RefreshCw size={14} />
                        杞崲
                      </Button>
                    </div>
                  ) : null}
                </div>
                <div className="provider-key-actions">
                  {existingProvider ? (
                    <LabeledSwitch
                      compact
                      label={key.enabled ? "鍚敤" : "绂佺敤"}
                      selected={key.enabled}
                      onChange={(enabled) => onToggleKey(key, enabled)}
                    />
                  ) : (
                    <Status ok text="寰呭惎鐢? />
                  )}
                  {existingProvider ? (
                    <>
                      <Button type="button" size="sm" variant="tertiary" onPress={() => onSaveName(key)}>
                        <Save size={14} />
                      </Button>
                      <Button type="button" size="sm" variant="tertiary" onPress={() => onRestore(key)}>
                        <RotateCcw size={14} />
                      </Button>
                    </>
                  ) : null}
                  <Button
                    type="button"
                    size="sm"
                    variant="tertiary"
                    isDisabled={index === 0}
                    aria-label="涓婄Щ"
                    onPress={() => onMove(index, -1)}
                  >
                    <ArrowUp size={14} />
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="tertiary"
                    isDisabled={index === keys.length - 1}
                    aria-label="涓嬬Щ"
                    onPress={() => onMove(index, 1)}
                  >
                    <ArrowDown size={14} />
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="danger"
                    aria-label="鍒犻櫎"
                    onPress={() => (existingProvider ? onDelete(key) : onPendingDelete(index))}
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
    return <div className="empty-state">鏆傛棤妯″瀷</div>;
  }
  return (
    <div className="table-wrap provider-models">
      <table className="data-table">
        <thead>
          <tr>
            <th>妯″瀷</th>
            <th>鑳藉姏</th>
            <th>鐘舵€?/th>
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
                    {model.supports_stream ? <span className="badge">娴佸紡</span> : null}
                  </div>
                </td>
                <td>
                  <div className="stack-tight">
                    {autoDisableEnabled && model.auto_disabled ? (
                      <span className="badge badge-danger">鑷姩绂佺敤</span>
                    ) : cooldownText ? (
                      <span className="badge badge-warning">{cooldownText}</span>
                    ) : model.enabled ? (
                      <Status ok text="鍚敤" />
                    ) : (
                      <Status text="绂佺敤" />
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
            <th>妯″瀷</th>
            <th>鑳藉姏</th>
            <th>涓婁笅鏂?/th>
            <th>鐘舵€?/th>
            <th>鎿嶄綔</th>
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
                      label="娴佸紡"
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
                      label={model.enabled ? "鍚敤" : "绂佺敤"}
                      selected={model.enabled}
                      onChange={(value) => updateModel(model, { enabled: value })}
                    />
                    {autoDisabledActive ? (
                      <span className="badge badge-danger">鑷姩绂佺敤</span>
                    ) : cooldownText ? (
                      <span className="badge badge-warning">{cooldownText}</span>
                    ) : !autoDisableOn && modelHasAutoState(model) ? (
                      <span className="badge badge-warning">鑷姩绂佺敤宸插叧闂?/span>
                    ) : !issueLabel ? (
                      <Status ok text="姝ｅ父" />
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
                      {model.auto_disabled ? "鎭㈠" : "娓呴櫎"}
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
  const [deletingGroup, setDeletingGroup] = useState<ModelGroup | null>(null);

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

  function confirmDeleteGroup() {
    if (!deletingGroup) return;
    const groupID = deletingGroup.id;
    setDeletingGroup(null);
    run(async () => del(`/api/admin/groups/${enc(groupID)}`));
  }

  return (
    <div className="surface">
      <div className="section row list-header">
        <div>
          <h2 style={{ margin: 0 }}>妯″瀷缁?/h2>

        </div>
        <Button onPress={openAdd}>
          <Plus size={16} />
          娣诲姞妯″瀷缁?        </Button>
      </div>

      <ListGroups
        groups={data.groups}
        edit={openEdit}
        remove={setDeletingGroup}
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
                    <Modal.Heading>{isEditing ? "缂栬緫妯″瀷缁? : "娣诲姞妯″瀷缁?}</Modal.Heading>

                  </Modal.Header>
                  <Modal.Body className="config-modal-body">
                    <div className="grid-2">
                      <TextField fullWidth isDisabled={isEditing} value={form.id} onChange={(id) => setForm({ ...form, id })}>
                        <Label>缁?ID</Label>
                        <Input placeholder="鐣欑┖鑷姩鐢熸垚锛屾垨濉啓 gpt-fast" />
                        <Description>淇濆瓨鍚庝笉鍙慨鏀?/Description>
                      </TextField>
                      <TextField fullWidth value={form.name} onChange={(name) => setForm({ ...form, name })}>
                        <Label>缁勫悕绉?/Label>
                        <Input placeholder="gpt 蹇€熺粍" />
                      </TextField>
                    </div>

                    <div className="row">
                      <AppSelect
                        className="select-field"
                        label="绛栫暐"
                        value={form.strategy}
                        onChange={(strategy) =>
                          setForm({ ...form, strategy: strategy as ModelGroup["strategy"] })
                        }
                        options={[
                          { id: "fallback", label: "鍥哄畾椤哄簭" },
                          { id: "random", label: "闅忔満" },
                          { id: "round_robin", label: "杞" },
                        ]}
                      />
                      <LabeledSwitch
                        label="鍚敤妯″瀷缁?
                        selected={form.enabled}
                        onChange={(enabled) => setForm({ ...form, enabled })}
                      />
                    </div>

                    <div className="config-editor">
                      <div className="config-add-row">
                        <AppSelect
                          className="select-field select-field-grow"
                          label="娣诲姞缁勫唴妯″瀷"
                          value={modelToAddValue}
                          onChange={setModelToAdd}
                          placeholder={availableModels.length === 0 ? "娌℃湁鍙坊鍔犵殑妯″瀷" : "閫夋嫨妯″瀷"}
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
                          鍔犲叆
                        </Button>
                      </div>

                      {form.members.length === 0 ? (
                        <div className="empty-state">鏆傛棤缁勫唴妯″瀷</div>
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
                                    <Status text="涓嶅彲鐢? />
                                  ) : (
                                    <Status ok text="鍙敤" />
                                  )}
                                </div>
                                <div className="row config-row-actions">
                                  <LabeledSwitch
                                    compact
                                    label={member.enabled ? "鍚敤" : "绂佺敤"}
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
                                    aria-label="涓婄Щ"
                                    onPress={() => setForm({ ...form, members: moveItem(form.members, index, -1) })}
                                  >
                                    <ArrowUp size={14} />
                                  </Button>
                                  <Button
                                    type="button"
                                    size="sm"
                                    variant="tertiary"
                                    isDisabled={index === form.members.length - 1}
                                    aria-label="涓嬬Щ"
                                    onPress={() => setForm({ ...form, members: moveItem(form.members, index, 1) })}
                                  >
                                    <ArrowDown size={14} />
                                  </Button>
                                  <Button
                                    type="button"
                                    size="sm"
                                    variant="danger"
                                    aria-label="绉婚櫎"
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
                      鍙栨秷
                    </Button>
                    <Button type="submit">
                      <Save size={16} />
                      淇濆瓨妯″瀷缁?                    </Button>
                  </Modal.Footer>
                </form>
              )}
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
      <DeleteConfirmModal
        isOpen={Boolean(deletingGroup)}
        title="鍒犻櫎妯″瀷缁?
        targetName={deletingGroup ? deletingGroup.name || deletingGroup.id : ""}
        description="鍒犻櫎鍚庝細绉婚櫎杩欎釜妯″瀷缁勫拰缁勫唴鎴愬憳銆備娇鐢ㄨ繖涓ā鍨嬬粍鐨勮矾鐢变笉浼氳嚜鍔ㄥ垹闄わ紝璇蜂箣鍚庢鏌ヨ矾鐢辩洰鏍囥€?
        onCancel={() => setDeletingGroup(null)}
        onConfirm={confirmDeleteGroup}
      />
    </div>
  );
}

function Routes({ data, run }: { data: AppData; run: (task: () => Promise<void>) => void }) {
  const [form, setForm] = useState<RouteFormState>(blankRouteForm);
  const [editingID, setEditingID] = useState("");
  const [open, setOpen] = useState(false);
  const [targetType, setTargetType] = useState<"model" | "group">("group");
  const [targetID, setTargetID] = useState("");
  const [deletingRoute, setDeletingRoute] = useState<Route | null>(null);

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
        throw new Error("璇峰～鍐欒櫄鎷熸ā鍨?ID");
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

  function confirmDeleteRoute() {
    if (!deletingRoute) return;
    const routeID = deletingRoute.id;
    setDeletingRoute(null);
    run(async () => del(`/api/admin/routes/${enc(routeID)}`));
  }

  return (
    <div className="surface">
      <div className="section row list-header">
        <div>
          <h2 style={{ margin: 0 }}>璺敱妯″瀷</h2>

        </div>
        <Button onPress={openAdd}>
          <Plus size={16} />
          娣诲姞璺敱
        </Button>
      </div>

      <ListRoutes
        routes={data.routes}
        groups={data.groups}
        models={data.models}
        edit={openEdit}
        remove={setDeletingRoute}
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
                    <Modal.Heading>{isEditing ? "缂栬緫璺敱" : "娣诲姞璺敱"}</Modal.Heading>

                  </Modal.Header>
                  <Modal.Body className="config-modal-body">
                    <div className="grid-2">
                      <TextField fullWidth isDisabled={isEditing} value={form.id} onChange={(id) => setForm({ ...form, id })}>
                        <Label>铏氭嫙妯″瀷 ID</Label>
                        <Input placeholder="coder-fast" />
                      </TextField>
                      <TextField fullWidth value={form.name} onChange={(name) => setForm({ ...form, name })}>
                        <Label>鏄剧ず鍚嶇О</Label>
                        <Input placeholder="coder-fast" />
                      </TextField>
                    </div>

                    <LabeledSwitch
                      label="鍚敤璺敱妯″瀷"
                      selected={form.enabled}
                      onChange={(enabled) => setForm({ ...form, enabled })}
                    />

                    <div className="config-editor">
                      <div className="config-add-row route-target-add">
                        <AppSelect
                          className="select-field"
                          label="鐩爣绫诲瀷"
                          value={targetType}
                          onChange={(type) => {
                            setTargetType(type as "model" | "group");
                            setTargetID("");
                          }}
                          options={[
                            { id: "group", label: "妯″瀷缁? },
                            { id: "model", label: "鍗曚釜妯″瀷" },
                          ]}
                        />
                        <AppSelect
                          className="select-field select-field-grow"
                          label="娣诲姞鐩爣"
                          value={targetIDValue}
                          onChange={setTargetID}
                          placeholder={availableTargets.length === 0 ? "娌℃湁鍙坊鍔犵殑鐩爣" : "閫夋嫨鐩爣"}
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
                          鍔犲叆
                        </Button>
                      </div>

                      {form.steps.length === 0 ? (
                        <div className="empty-state">鏆傛棤鐩爣</div>
                      ) : (
                        <div className="config-list">
                          {form.steps.map((step, index) => (
                            <div className="config-row" key={`${step.target_type}:${step.target_id}`}>
                              <div className="config-index">{index + 1}</div>
                              <div className="config-row-main">
                                <span className="badge">{step.target_type === "group" ? "妯″瀷缁? : "妯″瀷"}</span>
                                {step.target_id}
                              </div>
                              <div className="row config-row-actions">
                                <LabeledSwitch
                                  compact
                                  label={step.enabled ? "鍚敤" : "绂佺敤"}
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
                                  aria-label="涓婄Щ"
                                  onPress={() => setForm({ ...form, steps: moveItem(form.steps, index, -1) })}
                                >
                                  <ArrowUp size={14} />
                                </Button>
                                <Button
                                  type="button"
                                  size="sm"
                                  variant="tertiary"
                                  isDisabled={index === form.steps.length - 1}
                                  aria-label="涓嬬Щ"
                                  onPress={() => setForm({ ...form, steps: moveItem(form.steps, index, 1) })}
                                >
                                  <ArrowDown size={14} />
                                </Button>
                                <Button
                                  type="button"
                                  size="sm"
                                  variant="danger"
                                  aria-label="绉婚櫎"
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
                      鍙栨秷
                    </Button>
                    <Button type="submit">
                      <Save size={16} />
                      淇濆瓨璺敱
                    </Button>
                  </Modal.Footer>
                </form>
              )}
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
      <DeleteConfirmModal
        isOpen={Boolean(deletingRoute)}
        title="鍒犻櫎璺敱"
        targetName={deletingRoute ? deletingRoute.name || deletingRoute.id : ""}
        description="鍒犻櫎鍚庝細绉婚櫎杩欎釜铏氭嫙妯″瀷鍜屽畠鐨勭洰鏍囬『搴忋€傚鎴风灏嗕笉鑳藉啀浣跨敤杩欎釜铏氭嫙妯″瀷鍚嶃€?
        onCancel={() => setDeletingRoute(null)}
        onConfirm={confirmDeleteRoute}
      />
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
                    <span className="flow-node-meta">{group ? strategyLabel(group.strategy) : "妯″瀷缁?}</span>
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
    return <div className="surface section muted">鏆傛棤璺敱</div>;
  }

  return (
    <div className="surface">
      <div className="section row">
        <AppSelect
          className="select-field route-select"
          label="浠ｇ悊妯″瀷"
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
            <th>鏃堕棿</th>
            <th>鎺ュ彛</th>
            <th>妯″瀷</th>
            <th>缁撴灉</th>
            <th>灏濊瘯</th>
          </tr>
        </thead>
        <tbody>
          {logs.map((log) => (
            <tr key={log.id}>
              <td>{formatTime(log.created_at)}</td>
              <td>{log.api}</td>
              <td>
                <div className="code">{log.client_model}</div>
                <div className="muted">鏈€缁堬細{log.final_model || "-"}</div>
              </td>
              <td>
                <div className="stack">
                  {log.status === "success" ? (
                    <Status ok text={`${log.http_status} 路 ${log.duration_ms}ms`} />
                  ) : (
                    <Status text={`${log.http_status} 路 ${log.duration_ms}ms`} />
                  )}
                  {log.error ? <span className="error">{log.error}</span> : null}
                </div>
              </td>
              <td>
                <div className="stack">
                  {log.attempts?.map((attempt) => (
                    <div key={attempt.id}>
                      <span className="code">{attempt.model_id}</span>{" "}
                      {attempt.key_name ? (
                        <span className="badge">
                          {attempt.key_name} {attempt.key_prefix ? `路 ${attempt.key_prefix}...` : ""}
                        </span>
                      ) : null}{" "}
                      <span className="muted">
                        {attempt.status} 路 {attempt.http_status} 路 {attempt.duration_ms}ms
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
  const [keyName, setKeyName] = useState("榛樿瀹㈡埛绔?);
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
      setCopyError(err instanceof Error ? err.message : "鏄剧ず澶辫触锛岃绋嶅悗閲嶈瘯銆?);
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
      setCopyError(err instanceof Error ? err.message : "澶嶅埗澶辫触锛岃绋嶅悗閲嶈瘯銆?);
    }
  }

  return (
    <div className="surface">
      <div className="section stack">
        <h2>鏁呴殰鍒囨崲</h2>
        <LabeledSwitch
          label="鑷姩绂佺敤澶辫触妯″瀷"
          selected={autoDisableEnabled(data.settings)}
          onChange={(selected) =>
            run(async () => {
              await put("/api/admin/settings", { auto_disable_models: selected ? "true" : "false" });
            })
          }
        />
        <div className="row">
          <TextField value={upstreamTimeout} onChange={setUpstreamTimeout}>
            <Label>鍗曟ā鍨嬭秴鏃剁鏁?/Label>
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
            淇濆瓨
          </Button>
        </div>
      </div>

      <div className="section stack">
        <h2>/v1/models</h2>
        <LabeledSwitch
          label="鏆撮湶鍘熷妯″瀷"
          selected={data.settings.models_expose_raw === "true"}
          onChange={(selected) =>
            run(async () => {
              await put("/api/admin/settings", { models_expose_raw: selected ? "true" : "false" });
            })
          }
        />
        <div className="row">
          <TextField value={retention} onChange={setRetention}>
            <Label>鏃ュ織淇濈暀澶╂暟</Label>
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
            淇濆瓨
          </Button>
        </div>
      </div>

      <div className="section stack">
        <h2>浠ｇ悊璁块棶瀵嗛挜</h2>
        {copyError ? <div className="error">{copyError}</div> : null}
        <div className="row key-create-row">
          <TextField value={keyName} onChange={setKeyName}>
            <Label>瀵嗛挜鍚嶇О</Label>
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
            鍒涘缓瀵嗛挜
          </Button>
        </div>
        <table className="data-table">
          <thead>
            <tr>
              <th>鍚嶇О</th>
              <th>瀵嗛挜</th>
              <th>鐘舵€?/th>
              <th>鎿嶄綔</th>
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
                          {visibleToken ? "闅愯棌" : "鏄剧ず"}
                        </Button>
                        <Button size="sm" variant="secondary" onPress={() => copyToken(key.id)}>
                          {copied ? <Check size={14} /> : <Copy size={14} />}
                          {copied ? "宸插鍒? : "澶嶅埗"}
                        </Button>
                      </div>
                    </div>
                  </td>
                  <td>
                    <LabeledSwitch
                      compact
                      label={key.enabled ? "鍚敤" : "绂佺敤"}
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
        <h2>绠＄悊鍛樺瘑鐮?/h2>
        <div className="grid-2">
          <TextField type="password" value={oldPassword} onChange={setOldPassword}>
            <Label>鏃у瘑鐮?/Label>
            <Input />
          </TextField>
          <TextField type="password" value={newPassword} onChange={setNewPassword}>
            <Label>鏂板瘑鐮?/Label>
            <Input />
          </TextField>
        </div>
        <Button type="submit" variant="secondary">
          <Save size={16} />
          淇敼瀵嗙爜
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
  remove: (group: ModelGroup) => void;
}) {
  return (
    <div className="section stack">
      {groups.length === 0 ? (
        <div className="empty-state">鏆傛棤妯″瀷缁?/div>
      ) : null}
      {groups.map((group) => (
        <div className="row" key={group.id} style={{ justifyContent: "space-between" }}>
          <div>
            <strong>{group.name}</strong> <span className="code">{group.id}</span>
            <div className="muted">
              {strategyLabel(group.strategy)} 路 {group.members.length} 涓ā鍨?            </div>
            {group.members.length > 0 ? (
              <div className="muted list-preview">
                {group.members.slice(0, 3).map((member) => member.model_id).join(" 鈫?")}
                {group.members.length > 3 ? " ..." : ""}
              </div>
            ) : null}
          </div>
          <div className="row">
            <Button size="sm" variant="tertiary" onPress={() => edit(group)}>
              缂栬緫
            </Button>
            <Button size="sm" variant="danger" onPress={() => remove(group)}>
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
  remove: (route: Route) => void;
}) {
  return (
    <div className="section stack">
      {routes.length === 0 ? (
        <div className="empty-state">鏆傛棤璺敱</div>
      ) : null}
      {routes.map((route) => {
        const closedCount = routeClosedCount(route);
        const previewSteps = route.steps.slice(0, 3);
        return (
          <div className="row" key={route.id} style={{ justifyContent: "space-between" }}>
            <div>
              <strong>{route.name}</strong> <span className="code">{route.id}</span>
              <div className="muted">
                {route.steps.length} 涓洰鏍?路 {route.enabled ? "鍚敤" : "绂佺敤"} 路{" "}
                {closedCount > 0 ? `${closedCount} 椤瑰叧闂璥 : "鍏ㄩ儴鎵撳紑"}
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
                        {enabled ? "" : "锛堝叧闂級"}
                        {index < previewSteps.length - 1 ? " 鈫?" : ""}
                      </span>
                    );
                  })}
                  {route.steps.length > 3 ? " ..." : ""}
                </div>
              ) : null}
            </div>
            <div className="row">
              <Button size="sm" variant="tertiary" onPress={() => edit(route)}>
                缂栬緫
              </Button>
              <Button size="sm" variant="danger" onPress={() => remove(route)}>
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
      return "闅忔満";
    case "round_robin":
      return "杞";
    case "fallback":
    default:
      return "鍥哄畾椤哄簭";
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
  if (!routeModelProviderEnabled(model, providers)) return "鎻愪緵鍟嗘湭鍚敤";
  if (!model.enabled) return "鏈惎鐢?;
  if (autoDisableOn && model.auto_disabled) return "宸茬鐢?;
  if (autoDisableOn && modelCoolingDown(model)) return "鍐峰嵈涓?;
  return "";
}

function routeMemberStateLabel(
  member: ModelGroupMember,
  model: Model | undefined,
  providers: Provider[],
  autoDisableOn: boolean,
) {
  if (!member.enabled) return "鏈惎鐢?;
  return routeModelStateLabel(model, providers, autoDisableOn);
}

function routeModelProviderEnabled(model: Model, providers: Provider[]) {
  const provider = providers.find((item) => item.id === model.provider_id);
  if (provider) return provider.enabled;
  return model.provider_enabled !== false;
}

function routeGroupStateLabel(group: ModelGroup | undefined) {
  if (!group) return "";
  return group.enabled ? "" : "鏈惎鐢?;
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
  const label = `鍐峰嵈 ${Math.min(count, modelCooldownLimit)}/${modelCooldownLimit}`;
  if (!modelCoolingDown(model)) return label;
  return `${label} 路 ${modelCooldownRemaining(model)}`;
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
      return "瀵嗛挜寮傚父 401";
    case 403:
      return "鏉冮檺鍙楅檺 403";
    case 404:
      return "妯″瀷鎴栨帴鍙ｄ笉瀛樺湪 404";
    case 405:
      return "鎺ュ彛涓嶆敮鎸?405";
    case 410:
      return "妯″瀷宸蹭笅绾?410";
    default:
      return model.upstream_error_status > 0 ? `涓婃父 ${model.upstream_error_status}` : "";
  }
}

function modelIssueTitle(model: Model) {
  const parts = [
    model.upstream_error_at ? `鏃堕棿锛?{formatTime(model.upstream_error_at)}` : "",
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

function providerDeleteDescription(modelCount: number) {
  if (modelCount === 0) {
    return "鍒犻櫎鍚庝細绉婚櫎杩欎釜鎻愪緵鍟嗛厤缃€?;
  }
  return `鍒犻櫎鍚庝細绉婚櫎杩欎釜鎻愪緵鍟嗛厤缃紝骞跺悓鏃跺垹闄?${modelCount} 涓凡娣诲姞妯″瀷銆傜浉鍏虫ā鍨嬬粍鍜岃矾鐢辫涔嬪悗妫€鏌ャ€俙;
}

function DeleteConfirmModal({
  isOpen,
  title,
  targetName,
  description,
  onCancel,
  onConfirm,
}: {
  isOpen: boolean;
  title: string;
  targetName: string;
  description: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <Modal>
      <Modal.Backdrop
        isOpen={isOpen}
        onOpenChange={(open) => {
          if (!open) onCancel();
        }}
        variant="blur"
      >
        <Modal.Container size="sm">
          <Modal.Dialog className="confirm-dialog" role="alertdialog">
            {(dialog: { close: () => void }) => (
              <>
                <Modal.CloseTrigger />
                <Modal.Header>
                  <Modal.Heading>{title}</Modal.Heading>
                </Modal.Header>
                <Modal.Body className="confirm-body">
                  <p className="confirm-message">
                    纭鍒犻櫎 <span className="code confirm-target">{targetName}</span> 鍚楋紵
                  </p>
                  <p className="muted confirm-warning">姝ゆ搷浣滀笉鑳界洿鎺ユ挙閿€銆倇description}</p>
                </Modal.Body>
                <Modal.Footer>
                  <Button variant="tertiary" onPress={dialog.close}>
                    鍙栨秷
                  </Button>
                  <Button
                    variant="danger"
                    onPress={() => {
                      onConfirm();
                      dialog.close();
                    }}
                  >
                    <Trash2 size={16} />
                    纭鍒犻櫎
                  </Button>
                </Modal.Footer>
              </>
            )}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
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
  placeholder = "璇烽€夋嫨",
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
