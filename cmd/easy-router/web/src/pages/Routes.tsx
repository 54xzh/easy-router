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

export type RouteStepForm = {
  target_type: "model" | "group";
  target_id: string;
  enabled: boolean;
};

export type RouteFormState = {
  id: string;
  name: string;
  enabled: boolean;
  steps: RouteStepForm[];
};

export function blankRouteForm(): RouteFormState {
  return {
    id: "",
    name: "",
    enabled: true,
    steps: [],
  };
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
      <DeleteConfirmModal
        isOpen={Boolean(deletingRoute)}
        title="删除路由"
        targetName={deletingRoute ? deletingRoute.name || deletingRoute.id : ""}
        description="删除后会移除这个虚拟模型和它的目标顺序。客户端将不能再使用这个虚拟模型名。"
        onCancel={() => setDeletingRoute(null)}
        onConfirm={confirmDeleteRoute}
      />
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
export { Routes };
