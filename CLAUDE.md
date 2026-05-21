# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

cosweb is a lightweight, high-performance Go HTTP framework. It uses `cosgo/registry` for routing (static 16ns / parameterized 55ns lookup), sync.Pool context reuse, and nested Next-style middleware semantics. The project is a Go module at `github.com/hwcer/cosweb`.

Primary language is Go. Comments and documentation are in Chinese.

## Build & Test Commands

```bash
go build ./...
go test ./...
go test -run TestMiddlewareNestedSemantics   # run a single test
go vet ./...
```

No Makefile, no linter config, no CI pipeline — standard Go toolchain only.

## Architecture

### Request lifecycle

`Server.ServeHTTP` → acquire pooled `Context` → `Registry.Search(method, path)` to find route node → execute global middleware chain with handler at the tail → release Context back to pool.

The key method is `Context.doMiddlewareWithHandler`: it walks the global middleware slice, and when the chain is exhausted, calls `doHandle` on the matched `registry.Node`. This avoids allocating a new middleware slice or closure per request — `node` and `params` are stored directly on the Context.

### Two-level middleware

1. **Global middleware** (`Server.Use`) — runs on every request. The chain is stored as `srv.middleware []MiddlewareFunc`. Static file serving and reverse proxy are implemented as global middleware (not routes), so they can call `next()` to fall through to API routes when they don't match.

2. **Route-level middleware** (`Handler.Use`) — runs only for routes registered through a specific `Handler`/`Service`. When no route middleware exists, the handler is called directly (no slice/closure allocation).

Middleware signature: `func(*Context, Next) error`. Call `next()` to continue the chain; skip it to short-circuit. Errors returned from middleware go to `HTTPErrorHandler`.

### Handler pipeline

`Handler` wraps route execution with three optional hooks:
- **Filter** (`HandlerFilter`) — decides if a registered func/method/struct is a valid handler at registration time.
- **Caller** (`HandlerCaller`) — custom dispatch logic replacing the default reflection-based call.
- **Serialize** (`HandlerSerialize`) — custom response serialization replacing the default Accept-negotiated encoding.

Handler functions use the signature `func(*Context) any`. Returning an `error` from a handler triggers `HTTPErrorHandler`; any other return value is serialized via the Binder (JSON by default, negotiated from Accept/Content-Type headers).

### Static files and Proxy

Both `Static` and `Proxy` are global middleware, not routes. This is a deliberate design choice:
- **Static**: serves the file if it exists on disk; calls `next()` if not, falling through to API routes. Path traversal is guarded by `withinRoot`.
- **Proxy**: matches a URL prefix and forwards to upstream targets (random load balancing). `StripPrefix` controls whether the prefix is removed before forwarding. Non-matching requests call `next()`.

### Service / Registry integration

`Server.Service(name)` creates a `registry.Service` that can bulk-register struct methods as routes. The registry (from `cosgo/registry`) handles route matching and parameter extraction. The `Handler` attached to a Service acts as the pipeline for all routes in that Service.

### Key dependencies

- `github.com/hwcer/cosgo` — provides `registry` (route trie), `binder` (serialization), `scc` (graceful shutdown coordination), `values` (error types), `session`.
- `github.com/hwcer/logger` — structured logging.

### Context parameter resolution

`Context.Get(key)` searches multiple sources in priority order: Context store → path params → query → body → cookie. This order is configurable via `Server.RequestDataType`. Path params use `registry.Params.Get()` (linear scan, no map allocation). Query and body stores are lazily created on first access.

### Response model

`Response` wraps `http.ResponseWriter` with `written` and `hijacked` flags. `CanWrite()` returns false once any bytes have been written or the connection has been hijacked — `HTTPErrorHandler` and `Handler.write` check this to avoid double-writing.
