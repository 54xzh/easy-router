import React from "react";

export function Status({ ok, text }: { ok?: boolean; text: string }) {
  return <span className={`badge ${ok ? "badge-success" : "badge-danger"}`}>{text}</span>;
}
