import fs from 'fs';
import path from 'path';

const appPath = path.join(process.cwd(), 'src/App.tsx');
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
console.log("utils created!");
