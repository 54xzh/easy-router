import React from "react";
import { Chip } from "@heroui/react";

export function Status({ ok, text }: { ok?: boolean; text: string }) {
  return (
    <Chip
      size="sm"
      variant="soft"
      color={ok ? "success" : "danger"}
    >
      {text}
    </Chip>
  );
}
