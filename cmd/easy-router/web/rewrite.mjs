import fs from 'fs';
import path from 'path';

const appPath = path.join(process.cwd(), 'src/App.tsx');
const content = fs.readFileSync(appPath, 'utf8');
const lines = content.split('\n');

function getLines(start, end) {
    return lines.slice(start - 1, end).join('\n');
}

const imports = `import React, { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";
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

`;

const groupTypes = `export type GroupMemberForm = {
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

`;

const routeTypes = `export type RouteStepForm = {
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

`;

fs.writeFileSync('src/pages/Providers.tsx', imports + getLines(292, 844) + '\n' + getLines(845, 1029) + '\n' + getLines(1030, 1090) + '\nexport { Providers };\n');
fs.writeFileSync('src/pages/Models.tsx', imports + getLines(1091, 1235) + '\nexport { Models };\n');
fs.writeFileSync('src/pages/Groups.tsx', imports + groupTypes + getLines(1236, 1512) + '\n' + getLines(2311, 2352) + '\nexport { Groups };\n');
fs.writeFileSync('src/pages/Routes.tsx', imports + routeTypes + getLines(1513, 1800) + '\n' + getLines(2353, 2415) + '\nexport { Routes };\n');
fs.writeFileSync('src/pages/RouteSwitch.tsx', imports + getLines(1801, 2030) + '\nexport { RouteSwitch };\n');
fs.writeFileSync('src/pages/LogsView.tsx', imports + getLines(2031, 2088) + '\nexport { LogsView };\n');
fs.writeFileSync('src/pages/SettingsView.tsx', imports + getLines(2089, 2310) + '\nexport { SettingsView };\n');

console.log("Rewrite done");
