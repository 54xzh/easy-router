import React, { useState } from "react";
import { Card, TextField, Label, Input, Button, Separator } from "@heroui/react";
import { Shield, Cable } from "lucide-react";

export function Login({
  error,
  onLogin,
}: {
  error: string;
  onLogin: (username: string, password: string) => Promise<void>;
}) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");

  return (
    <div className="login-page">
      <Card className="login-box" variant="default">
        <Card.Header className="gap-3">
          <div className="brand-mark">
            <Cable size={20} />
          </div>
          <div>
            <Card.Title>登录 Easy Router</Card.Title>
            <Card.Description>LLM 代理路由管理平台</Card.Description>
          </div>
        </Card.Header>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            onLogin(username, password);
          }}
        >
          <Card.Content className="flex flex-col gap-4">
            {error ? <div className="error">{error}</div> : null}
            <TextField fullWidth value={username} onChange={setUsername}>
              <Label>用户名</Label>
              <Input placeholder="admin" />
            </TextField>
            <TextField fullWidth type="password" value={password} onChange={setPassword}>
              <Label>密码</Label>
              <Input placeholder="输入管理员密码" />
            </TextField>
          </Card.Content>
          <Separator />
          <Card.Footer className="flex flex-col gap-3">
            <Button type="submit" className="w-full">
              <Shield size={16} />
              登录
            </Button>
            <p className="muted" style={{ fontSize: 12, textAlign: "center" }}>
              首次启动密码会显示在后端控制台
            </p>
          </Card.Footer>
        </form>
      </Card>
    </div>
  );
}
