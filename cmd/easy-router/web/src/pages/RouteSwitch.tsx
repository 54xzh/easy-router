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

export { RouteSwitch };
