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

export { Models };
