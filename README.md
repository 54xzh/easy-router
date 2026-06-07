# Easy Router

Easy Router 是一个本地 LLM Proxy。客户端只需要调用一个虚拟模型，后端会按你配置的路由选择真实模型。

## 功能

- 支持 OpenAI 兼容提供商。
- 内部模型 ID 固定为 `provider_id/original_model_id`。
- 支持模型组：固定顺序、随机、轮询。
- 支持路由模型多层切换。
- 支持 `/v1/chat/completions`、`/v1/responses`、`/v1/models`。
- 管理后台使用 React、HeroUI v3 和 React Flow。
- SQLite 使用纯 Go 驱动。
- 提供商 API Key 使用 `EASY_ROUTER_SECRET_KEY` 加密保存。

## 配置

可以在项目根目录新建 `.env`：

```env
EASY_ROUTER_SECRET_KEY=请换成足够长的随机字符串
EASY_ROUTER_ADDR=127.0.0.1:2778
EASY_ROUTER_DB=./data/easy-router.db
```

只有 `EASY_ROUTER_SECRET_KEY` 是必填项。

也可以继续使用系统环境变量。系统环境变量优先级更高，会覆盖 `.env` 中的同名配置。

## 开发运行

先安装前端依赖并构建静态文件：

```powershell
cd cmd/easy-router/web
npm install
npm run build
```

然后回到项目根目录运行后端：

```powershell
cd ../../..
go run ./cmd/easy-router
```

如果 Windows 上 `go` 不在 PATH，可以用完整路径：

```powershell
& "C:\Program Files\Go\bin\go.exe" run ./cmd/easy-router
```

首次启动会在控制台显示管理员账号和密码。登录后请尽快修改密码。

## 客户端调用

先在后台“设置”里创建代理访问密钥，然后调用：

```bash
curl http://127.0.0.1:2778/v1/models \
  -H "Authorization: Bearer <proxy_key>"
```

Chat Completions 示例：

```bash
curl http://127.0.0.1:2778/v1/chat/completions \
  -H "Authorization: Bearer <proxy_key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"coder-fast","messages":[{"role":"user","content":"hello"}]}'
```

## 目录

- `cmd/easy-router`：后端入口和嵌入的前端静态文件。
- `internal/store`：SQLite 表结构和数据访问。
- `internal/proxy`：OpenAI 兼容代理、路由和失败后切换。
- `internal/admin`：管理后台 API。
- `cmd/easy-router/web`：React 管理后台。
