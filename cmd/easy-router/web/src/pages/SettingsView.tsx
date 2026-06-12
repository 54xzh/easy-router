import React, { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";
import { Background, Controls, Edge, MarkerType, Node, Position, ReactFlow } from "@xyflow/react";
import { Activity, ArrowDown, ArrowUp, Boxes, Cable, Check, ChevronDown, ChevronRight, Copy, Download, Edit3, Eye, EyeOff, GitBranch, KeyRound, ListTree, Logs, Plus, RefreshCw, RotateCcw, Save, Settings as SettingsIcon, Shield, Trash2 } from "lucide-react";
import { Button, Description, Input, Label, ListBox, Modal, Select, Switch, TextArea, TextField } from "@heroui/react";
import { api, del, enc, patch, post, put } from "../api";
import { AppData, Model, ModelGroup, ModelGroupMember, Provider, ProviderKey, ProxyKey, RequestLog, Route, RouteStep, Settings, RemoteModel, TabKey } from "../types";
import { LabeledSwitch } from "../components/LabeledSwitch";
import { Status } from "../components/Status";
import { AppSelect, SelectOption } from "../components/AppSelect";
import { DeleteConfirmModal } from "../components/DeleteConfirmModal";
import { FlowBox } from "../components/FlowBox";
import { moveItem, formatTime, copyText, strategyLabel, modelName, groupName, routeModelStateLabel, routeMemberStateLabel, routeModelProviderEnabled, routeGroupStateLabel, targetKey, routeOverrideDisabled, routeStepEnabled, routeClosedCount, routeTargetID, routeTargetName, routeStepName, autoDisableEnabled, modelCooldownLimit, modelCooldownLabel, modelCoolingDown, modelCooldownRemaining, modelHasAutoState, modelHasClearableState, modelIssueLabel, modelIssueTitle, replaceItem, removeItem, providerDeleteDescription } from "../utils";

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

export { SettingsView };
