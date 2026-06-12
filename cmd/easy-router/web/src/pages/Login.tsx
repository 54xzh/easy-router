import React, { useState } from "react";
import { TextField, Label, Input, Button } from "@heroui/react";
import { Shield } from "lucide-react";

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
      <form
        className="login-box surface section stack"
        onSubmit={(event) => {
          event.preventDefault();
          onLogin(username, password);
        }}
      >
        <div>
          <h1 className="page-title">登录 Easy Router</h1>
          <div className="page-subtitle">首次启动密码会显示在后端控制台。</div>
        </div>
        {error ? <div className="error">{error}</div> : null}
        <TextField fullWidth value={username} onChange={setUsername}>
          <Label>用户名</Label>
          <Input placeholder="admin" />
        </TextField>
        <TextField fullWidth type="password" value={password} onChange={setPassword}>
          <Label>密码</Label>
          <Input placeholder="输入管理员密码" />
        </TextField>
        <Button type="submit">
          <Shield size={16} />
          登录
        </Button>
      </form>
    </div>
  );
}
