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

export type GroupMemberForm = {
  model_id: string;
  enabled: boolean;
};

export type GroupFormState = {
  id: string;
  name: string;
  strategy: ModelGroup["strategy"];
  enabled: boolean;
  members: GroupMemberForm[];
};

export function blankGroupForm(): GroupFormState {
  return {
    id: "",
    name: "",
    strategy: "fallback",
    enabled: true,
    members: [],
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
      <DeleteConfirmModal
        isOpen={Boolean(deletingGroup)}
        title="删除模型组"
        targetName={deletingGroup ? deletingGroup.name || deletingGroup.id : ""}
        description="删除后会移除这个模型组和组内成员。使用这个模型组的路由不会自动删除，请之后检查路由目标。"
        onCancel={() => setDeletingGroup(null)}
        onConfirm={confirmDeleteGroup}
      />
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
            <Button size="sm" variant="danger" onPress={() => remove(group)}>
              <Trash2 size={14} />
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}

export { Groups };
