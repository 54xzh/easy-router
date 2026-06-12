import React from "react";
import { Switch, Label } from "@heroui/react";

export function LabeledSwitch({
  label,
  selected,
  onChange,
  compact,
}: {
  label: string;
  selected: boolean;
  onChange: (selected: boolean) => void;
  compact?: boolean;
}) {
  return (
    <Switch isSelected={selected} size={compact ? "sm" : "md"} onChange={onChange}>
      <Switch.Control>
        <Switch.Thumb />
      </Switch.Control>
      {label ? (
        <Switch.Content>
          <Label className={compact ? "text-xs" : "text-sm"}>{label}</Label>
        </Switch.Content>
      ) : null}
    </Switch>
  );
}
