import React, { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";
import { Background, Controls, Edge, MarkerType, Node, Position, ReactFlow } from "@xyflow/react";
import { Activity, ArrowDown, ArrowUp, Boxes, Cable, Check, ChevronDown, ChevronRight, Copy, Download, Edit3, Eye, EyeOff, GitBranch, KeyRound, ListTree, Logs, Plus, RefreshCw, RotateCcw, Save, Settings as SettingsIcon, Shield, Trash2 } from "lucide-react";
import { Button, Description, Input, Label, ListBox, Modal, Select, Switch, TextArea, TextField, Chip } from "@heroui/react";
import { api, del, enc, patch, post, put } from "../api";
import { AppData, Model, ModelGroup, ModelGroupMember, Provider, ProviderKey, ProxyKey, RequestLog, Route, RouteStep, Settings, RemoteModel, TabKey } from "../types";
import { LabeledSwitch } from "../components/LabeledSwitch";
import { Status } from "../components/Status";
import { AppSelect, SelectOption } from "../components/AppSelect";
import { DeleteConfirmModal } from "../components/DeleteConfirmModal";
import { FlowBox } from "../components/FlowBox";
import { moveItem, formatTime, copyText, strategyLabel, modelName, groupName, routeModelStateLabel, routeMemberStateLabel, routeModelProviderEnabled, routeGroupStateLabel, targetKey, routeOverrideDisabled, routeStepEnabled, routeClosedCount, routeTargetID, routeTargetName, routeStepName, autoDisableEnabled, modelCooldownLimit, modelCooldownLabel, modelCoolingDown, modelCooldownRemaining, modelHasAutoState, modelHasClearableState, modelIssueLabel, modelIssueTitle, replaceItem, removeItem, providerDeleteDescription } from "../utils";

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
                      {attempt.key_name ? (
                        <Chip size="sm" variant="secondary">
                          {attempt.key_name} {attempt.key_prefix ? `· ${attempt.key_prefix}...` : ""}
                        </Chip>
                      ) : null}{" "}
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

export { LogsView };
