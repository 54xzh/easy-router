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
import { AppData, Model, ModelGroup, ModelGroupMember, Provider, ProviderKey, ProxyKey, RequestLog, Route, RouteStep, Settings, RemoteModel, SelectOption } from "../types";
import { LabeledSwitch } from "../components/LabeledSwitch";
import { Status } from "../components/Status";
import { AppSelect } from "../components/AppSelect";
import { DeleteConfirmModal } from "../components/DeleteConfirmModal";
import { FlowBox } from "../components/FlowBox";
import { moveItem, formatTime, copyText } from "../utils";

function autoDisableEnabled(settings: Settings) {
  return settings.auto_disable_models === "true";
}

function strategyLabel(strategy: string) {
  switch (strategy) {
    case "random":
      return "随机";
    case "fallback":
      return "兜底";
    case "round_robin":
    default:
      return "轮询";
  }
}

function replaceItem<T>(items: T[], index: number, item: T) {
  const next = [...items];
  next[index] = item;
  return next;
}

function removeItem<T>(items: T[], index: number) {
  const next = [...items];
  next.splice(index, 1);
  return next;
}

`;

fs.writeFileSync('src/pages/Providers.tsx', imports + getLines(292, 844) + '\n' + getLines(845, 1029) + '\n' + getLines(1030, 1090));
fs.writeFileSync('src/pages/Models.tsx', imports + getLines(1091, 1235));
fs.writeFileSync('src/pages/Groups.tsx', imports + getLines(1236, 1512) + '\n' + getLines(2311, 2352));
fs.writeFileSync('src/pages/Routes.tsx', imports + getLines(1513, 1800) + '\n' + getLines(2353, 2579));
fs.writeFileSync('src/pages/RouteSwitch.tsx', imports + getLines(1801, 2030));
fs.writeFileSync('src/pages/LogsView.tsx', imports + getLines(2031, 2088));
fs.writeFileSync('src/pages/SettingsView.tsx', imports + getLines(2089, 2310));

console.log("Splitting done");
