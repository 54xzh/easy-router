import React, { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";
import { Background, Controls, Edge, MarkerType, Node, Position, ReactFlow } from "@xyflow/react";
import { Activity, ArrowDown, ArrowUp, Boxes, Cable, Check, ChevronDown, ChevronRight, Copy, Download, Edit3, Eye, EyeOff, GitBranch, KeyRound, ListTree, Logs, Plus, RefreshCw, RotateCcw, Save, Settings as SettingsIcon, Shield, Trash2 } from "lucide-react";
import { Button, Description, Input, Label, ListBox, Modal, Select, Switch, TextArea, TextField, Chip, Separator } from "@heroui/react";
import { api, del, enc, patch, post, put } from "../api";
import { AppData, Model, ModelGroup, ModelGroupMember, Provider, ProviderKey, ProxyKey, RequestLog, Route, RouteStep, Settings, RemoteModel, TabKey } from "../types";
import { LabeledSwitch } from "../components/LabeledSwitch";
import { Status } from "../components/Status";
import { AppSelect, SelectOption } from "../components/AppSelect";
import { DeleteConfirmModal } from "../components/DeleteConfirmModal";
import { FlowBox } from "../components/FlowBox";
import { moveItem, formatTime, copyText, strategyLabel, modelName, groupName, routeModelStateLabel, routeMemberStateLabel, routeModelProviderEnabled, routeGroupStateLabel, targetKey, routeOverrideDisabled, routeStepEnabled, routeClosedCount, routeTargetID, routeTargetName, routeStepName, autoDisableEnabled, modelCooldownLimit, modelCooldownLabel, modelCoolingDown, modelCooldownRemaining, modelHasAutoState, modelHasClearableState, modelIssueLabel, modelIssueTitle, replaceItem, removeItem, providerDeleteDescription } from "../utils";

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
      setKeyError(err instanceof Error ? err.message : "Key 列表加载失败");
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
      setKeyError("请先填写 API Key");
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
                  {provider.multi_key_enabled ? (
                    <Chip size="sm" variant="secondary">
                      多 Key {provider.enabled_key_count ?? 0}/{provider.key_count ?? 0} · {strategyLabel(provider.multi_key_strategy)}
                    </Chip>
                  ) : null}
                  <Button size="sm" variant="secondary" onPress={() => openEdit(provider)}>
                    <Edit3 size={14} />
                    编辑
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
                            {model.already_imported ? <Chip size="sm" variant="secondary">已添加</Chip> : null}
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
                    {existingProvider ? "编辑提供商" : "添加提供商"}
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
                          <Label>提供商 ID</Label>
                          <Input placeholder="openai" />
                          <Description>保存后不可修改</Description>
                        </TextField>
                        <TextField fullWidth value={form.name} onChange={(name) => setForm({ ...form, name })}>
                          <Label>显示名称</Label>
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
                        <Input placeholder={form.multi_key_enabled ? "由右侧多 Key 管理" : "编辑时留空表示不修改"} />
                      </TextField>
                      <TextField fullWidth value={headersText} onChange={setHeadersText}>
                        <Label>额外请求头 JSON</Label>
                        <TextArea rows={3} placeholder='{"HTTP-Referer":"http://localhost"}' />
                      </TextField>
                      <div className="provider-switches">
                        <LabeledSwitch
                          label="启用提供商"
                          selected={form.enabled}
                          onChange={(enabled) => setForm({ ...form, enabled })}
                        />
                        <LabeledSwitch
                          label="启用多 Key"
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
      <DeleteConfirmModal
        isOpen={Boolean(deletingProvider)}
        title="删除提供商"
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
          <h3>多 Key 管理</h3>
          <div className="muted">{existingProvider ? `${keys.length} 个 Key` : "保存时创建这些 Key"}</div>
        </div>
        <AppSelect
          className="select-field provider-key-strategy"
          label="请求策略"
          value={strategy}
          onChange={(value) => onStrategyChange(value as Provider["multi_key_strategy"])}
          options={[
            { id: "round_robin", label: "轮询" },
            { id: "fallback", label: "固定顺序" },
            { id: "random", label: "随机" },
          ]}
        />
      </div>

      {keyError ? <div className="error">{keyError}</div> : null}

      <div className="provider-key-add">
        <TextField value={newKeyName} onChange={onNewKeyNameChange}>
          <Label>Key 名称</Label>
          <Input placeholder={`Key ${keys.length + 1}`} />
        </TextField>
        <TextField type="password" value={newKeyValue} onChange={onNewKeyValueChange}>
          <Label>API Key</Label>
          <Input placeholder="sk-..." />
        </TextField>
        <Button type="button" variant="secondary" onPress={onAddKey}>
          <Plus size={16} />
          添加
        </Button>
      </div>

      {keys.length === 0 ? (
        <div className="empty-state">暂无 Key</div>
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
                    <span className="code">{key.prefix ? `${key.prefix}...` : "待保存"}</span>
                    {key.model_issue_count > 0 ? (
                      <Chip size="sm" variant="soft" color="warning">{key.model_issue_count} 个异常模型</Chip>
                    ) : (
                      <Chip size="sm" variant="soft" color="success">状态正常</Chip>
                    )}
                  </div>
                  {existingProvider ? (
                    <div className="provider-key-rotate">
                      <input
                        type="password"
                        value={rotateValues[key.id] ?? ""}
                        onChange={(event) => onRotateValueChange(key.id, event.target.value)}
                        placeholder="填写新 API Key 后轮换"
                      />
                      <Button type="button" size="sm" variant="secondary" onPress={() => onRotate(key)}>
                        <RefreshCw size={14} />
                        轮换
                      </Button>
                    </div>
                  ) : null}
                </div>
                <div className="provider-key-actions">
                  {existingProvider ? (
                    <LabeledSwitch
                      compact
                      label={key.enabled ? "启用" : "禁用"}
                      selected={key.enabled}
                      onChange={(enabled) => onToggleKey(key, enabled)}
                    />
                  ) : (
                    <Status ok text="待启用" />
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
                    aria-label="上移"
                    onPress={() => onMove(index, -1)}
                  >
                    <ArrowUp size={14} />
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="tertiary"
                    isDisabled={index === keys.length - 1}
                    aria-label="下移"
                    onPress={() => onMove(index, 1)}
                  >
                    <ArrowDown size={14} />
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="danger"
                    aria-label="删除"
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
                    {model.supports_chat ? <Chip size="sm" variant="soft" color="success">Chat</Chip> : null}
                    {model.supports_responses ? <Chip size="sm" variant="secondary">Responses</Chip> : null}
                    {model.supports_stream ? <Chip size="sm" variant="secondary">流式</Chip> : null}
                  </div>
                </td>
                <td>
                  <div className="stack-tight">
                    {autoDisableEnabled && model.auto_disabled ? (
                      <Chip size="sm" variant="soft" color="danger">自动禁用</Chip>
                    ) : cooldownText ? (
                      <Chip size="sm" variant="soft" color="warning">{cooldownText}</Chip>
                    ) : model.enabled ? (
                      <Status ok text="启用" />
                    ) : (
                      <Status text="禁用" />
                    )}
                    {issueLabel ? (
                      <Chip size="sm" variant="soft" color="warning" title={modelIssueTitle(model)}>
                        {issueLabel}
                      </Chip>
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

export { Providers };
