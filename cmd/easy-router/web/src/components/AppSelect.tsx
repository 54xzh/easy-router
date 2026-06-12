import React, { ReactNode } from "react";
import { Select, Label, ListBox } from "@heroui/react";

export type SelectOption = {
  id: string;
  label: ReactNode;
  textValue?: string;
};

export function AppSelect({
  label,
  value,
  onChange,
  options,
  placeholder = "请选择",
  className,
  isDisabled,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  className?: string;
  isDisabled?: boolean;
}) {
  return (
    <Select
      className={className}
      fullWidth
      isDisabled={isDisabled || options.length === 0}
      placeholder={placeholder}
      value={value || null}
      variant="secondary"
      onChange={(nextValue) => onChange(nextValue === null ? "" : String(nextValue))}
    >
      <Label>{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="app-select-popover">
        <ListBox>
          {options.map((option) => (
            <ListBox.Item id={option.id} key={option.id} textValue={option.textValue ?? String(option.label)}>
              <span className="select-option-label">{option.label}</span>
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  );
}
