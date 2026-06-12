import fs from 'fs';
import path from 'path';

const appPath = path.join(process.cwd(), 'App.bak.tsx');
const content = fs.readFileSync(appPath, 'utf8');
const lines = content.split('\n');

function getLines(start, end) {
    return lines.slice(start - 1, end).join('\n');
}

const utilsCode = getLines(2412, 2573)
  .replace(/^function /gm, 'export function ')
  .replace(/^const /gm, 'export const ');

const existingUtils = `import { Model, ModelGroup, Provider, Route, RouteStep, Settings, AppData } from "./types";
import { RouteStepForm } from "./pages/Routes";
import { GroupMemberForm } from "./pages/Groups";

export function moveItem<T>(items: T[], index: number, delta: number) {
  const nextIndex = index + delta;
  if (nextIndex < 0 || nextIndex >= items.length) return items;
  const next = [...items];
  const [item] = next.splice(index, 1);
  next.splice(nextIndex, 0, item);
  return next;
}

export function formatTime(value: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

export async function copyText(text: string) {
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

`;

fs.writeFileSync('src/utils.ts', existingUtils + utilsCode);

// Also modify split.mjs to use these utils instead of inline
const splitScript = `import fs from 'fs';
import path from 'path';

const appPath = path.join(process.cwd(), 'App.bak.tsx');
const content = fs.readFileSync(appPath, 'utf8');
const lines = content.split('\\n');

function getLines(start, end) {
    return lines.slice(start - 1, end).join('\\n');
}

const imports = \`import React, { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";
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

\`;

const routeAndGroupTypes = \`export type GroupMemberForm = {
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

\`;

fs.writeFileSync('src/pages/Providers.tsx', imports + getLines(292, 844) + '\\n' + getLines(845, 1029) + '\\n' + getLines(1030, 1090) + '\\nexport { Providers };\\n');
fs.writeFileSync('src/pages/Models.tsx', imports + getLines(1091, 1235) + '\\nexport { Models };\\n');
fs.writeFileSync('src/pages/Groups.tsx', imports + routeAndGroupTypes + getLines(1236, 1512) + '\\n' + getLines(2311, 2352) + '\\nexport { Groups };\\n');
fs.writeFileSync('src/pages/Routes.tsx', imports + routeAndGroupTypes + getLines(1513, 1800) + '\\n' + getLines(2353, 2579) + '\\nexport { Routes };\\n');
fs.writeFileSync('src/pages/RouteSwitch.tsx', imports + getLines(1801, 2030) + '\\nexport { RouteSwitch };\\n');
fs.writeFileSync('src/pages/LogsView.tsx', imports + getLines(2031, 2088) + '\\nexport { LogsView };\\n');
fs.writeFileSync('src/pages/SettingsView.tsx', imports + getLines(2089, 2310) + '\\nexport { SettingsView };\\n');

console.log("Rewrite done");
\`;

fs.writeFileSync('split2.mjs', splitScript);
console.log("update_utils done");
