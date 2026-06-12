import React, { ReactNode } from "react";

export function FlowBox({
  children,
  className,
  disabled,
}: {
  children: ReactNode;
  className?: string;
  disabled?: boolean;
}) {
  return <div className={`flow-node ${className ?? ""} ${disabled ? "flow-node-disabled" : ""}`}>{children}</div>;
}
