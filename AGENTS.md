# AGENTS.md

## Project Overview

Easy Router is a local LLM proxy. Clients call one virtual model name, and the backend routes the request to real OpenAI-compatible provider models.

The project has two main parts:

- Go backend in `cmd/easy-router` and `internal`.
- React admin UI in `cmd/easy-router/web`.

The built frontend lives in `cmd/easy-router/web/dist` and is embedded into the Go binary with `go:embed`.

## Main Directories

- `cmd/easy-router`: application entry point and embedded web assets.
- `cmd/easy-router/web`: React admin UI built with Vite, HeroUI v3, React Flow, and TypeScript.
- `internal/config`: environment and `.env` loading.
- `internal/store`: SQLite schema, migrations, data access, encryption, keys, logs, providers, models, groups, and routes.
- `internal/proxy`: OpenAI-compatible proxy endpoints, routing, fallback, streaming, and request logging.
- `internal/admin`: admin API used by the web UI.
- `data`: local SQLite database location during development.
- `bin`: local build output.

## Common Commands

Run commands from the repository root unless noted.

Prefer the repository scripts below. Avoid adding platform-specific commands unless the task truly needs them.

Install frontend dependencies:

```sh
npm run web:install
```

Build the frontend only:

```sh
npm run web:build
```

Build frontend plus backend binary:

```sh
npm run build
```

Run all Go tests:

```sh
npm test
```

Run the backend during development after the frontend has been built:

```sh
go run ./cmd/easy-router
```

## Required Environment

`EASY_ROUTER_SECRET_KEY` is required. It is used to encrypt stored provider API keys and proxy keys.

A typical local `.env` file:

```env
EASY_ROUTER_SECRET_KEY=replace-with-a-long-random-string
EASY_ROUTER_ADDR=127.0.0.1:2778
EASY_ROUTER_DB=./data/easy-router.db
```

System environment variables override values from `.env`.

Do not commit real secrets, local databases, or runtime logs.

## Backend Guidelines

- Keep the public proxy API compatible with OpenAI-style endpoints:
  - `/v1/models`
  - `/v1/chat/completions`
  - `/v1/responses`
- Preserve the internal model ID format: `provider_id/original_model_id`.
- Keep fallback behavior predictable. Failed upstream calls may move to the next candidate model when the status is fallback-safe.
- Treat streaming carefully. Once a streaming response starts, the handler cannot safely switch to another upstream response.
- Keep request logging accurate. Update `RequestLog` and `AttemptLog` behavior together when changing proxy flow.
- Keep provider API keys encrypted through `EASY_ROUTER_SECRET_KEY`.
- Use the existing SQLite migration style in `internal/store/store.go`.
- Use `gofmt` on changed Go files.

## Frontend Guidelines

- The admin UI uses React, TypeScript, HeroUI v3, React Flow, and Vite.
- Use `npm`, not another package manager. The web app has a `package-lock.json`.
- When changing the admin UI, run `npm run web:build` so `cmd/easy-router/web/dist` is refreshed for Go embedding.
- Prefer existing component patterns in `cmd/easy-router/web/src/App.tsx` and API helpers in `cmd/easy-router/web/src/api.ts`.
- Use `lucide-react` icons where an icon button is needed.
- Keep operational screens clear and compact. This is an admin tool, not a marketing site.

## Testing Expectations

- For backend behavior changes, add or update Go tests near the changed package.
- Existing test areas include:
  - `internal/config/config_test.go`
  - `internal/store/store_test.go`
  - `internal/proxy/proxy_test.go`
- Run `npm test` before finishing backend changes.
- Run `npm run web:build` before finishing frontend changes.
- For changes that affect the final binary or embedded UI, run `npm run build`.

## Binary Build Expectations

After code changes are complete, build release binaries for at least these targets:

- Windows x64: `GOOS=windows`, `GOARCH=amd64`, output `bin/easy-router-windows-amd64.exe`.
- Linux aarch64: `GOOS=linux`, `GOARCH=arm64`, output `bin/easy-router-linux-arm64`.

Use the equivalent environment variable syntax for the current shell. Do not assume the development machine is one of the target platforms.

## Language And Copy

- This instruction file is written in English.
- Existing user-facing app text is mostly Simplified Chinese. Keep that style unless the task asks for a different language.
- Keep error messages short and actionable.

## Git And Generated Files

- Do not overwrite user changes.
- Do not remove `cmd/easy-router/web/dist` unless the task explicitly asks for it; the Go binary embeds this folder.
- Avoid committing local files such as `.env`, `data/*.db`, `bin/*`, and `*.log`.
- Commit titles and commit descriptions should be in Chinese unless they contain proper names.
- If a commit description has multiple points, use an unordered list.
