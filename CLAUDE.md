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

`Server.ServeHTTP` → acquire pooled `Context` → `Registry.Search(method, path)` to find route node → assemble middleware chain (global + route-level) → `Context.doDispatch()` walks the chain → handler runs at the tail → release Context back to pool.

`Server.ServeHTTP` has three branch paths after route lookup:
1. **Node matched** (`c.node != nil`): If the node's handler is a `*Handler`, its route-level middleware is appended to the global middleware chain (copied into a new slice to avoid mutating the global slice).
2. **No node, but Service exists** (`service != nil`): Graceful fallback — the Service's `*Handler` is used with its middleware, even though no specific route matched. This enables Service-level middleware/meta-routes.
3. **Neither**: Falls through to `doDispatch` with only global middleware, which returns `ErrNotFound`.

Graceful shutdown is coordinated via `scc` (from `cosgo`): `scc.Add(1)`/`scc.Done()` tracks in-flight requests, `scc.Trigger(srv.shutdown)` registers the shutdown callback, and startup methods use `scc.Timeout` to avoid blocking on `ListenAndServe`.

### Dispatch and middleware chain

The `dispatch` struct (stored inline on `Context` to avoid heap allocation) holds the current state of middleware chain execution:

- `index` — current position in `funcs`
- `funcs` — the merged middleware slice (global + route-level)
- `handler` — the `*Handler` that will invoke the matched route
- `nexts` — remaining fallback routes (populated lazily by `c.Next()`)

`Context.doDispatch()` walks `funcs[index:]`, calling each middleware with the cached method value `c.dispatchFn` (avoids allocating a closure per call). When the chain is exhausted, it calls `handler.handle(node, c)` then `handler.write(c, reply)`.

`Context.dispatchFn` is pre-bound at pool creation (`c.dispatchFn = c.doDispatch`) — this is a method value, not a closure, so it allocates once at pool init, not per request.

### Multi-route fallback (`c.Next()`)

When a handler or middleware calls `c.Next()`, it searches for the **next** matching route via `Registry.SearchAll()` (returns all routes matching method+path in priority order). The first match was already consumed; `Next()` shifts to the next one, resets `node`/`params`, merges any new route-level middleware, and re-invokes `doDispatch()`.

This is how Static and Proxy fall through: they are registered as wildcard routes (`/prefix/*`), and if they can't serve the request (file not found, no matching upstream), they call `c.Next()` to try the next route.

### Two-level middleware

1. **Global middleware** (`Server.Use`) — runs on every request. Stored as `srv.middleware []MiddlewareFunc`.

2. **Route-level middleware** (`Handler.Use`) — runs only for routes registered through a specific `Handler`/`Service`. When no route middleware exists, the handler is called directly (no slice/closure allocation). When route middleware exists, a new slice is allocated that copies global + route middleware — the original global slice is never mutated.

Middleware signature: `func(*Context, Next) error`. Call `next()` to continue the chain; skip it to short-circuit. `Next` is `func() error` — the `dispatchFn` method value.

### Handler pipeline

`Handler` wraps route execution with three optional hooks:
- **Filter** (`HandlerFilter`) — decides if a registered func/method/struct is a valid handler at registration time. Default: accepts `func(*Context) any` for funcs, 2-in/1-out methods for struct methods, and structs implementing `handleCaller`.
- **Caller** (`HandlerCaller`) — custom dispatch logic replacing the default reflection-based call. Structs that implement `handleCaller` (via `Caller(node, c)`) avoid reflection overhead.
- **Serialize** (`HandlerSerialize`) — custom response serialization replacing the default Accept-negotiated encoding.

Handler functions use the signature `func(*Context) any`. Returning an `error` (or `*HTTPError`) from a handler triggers `HTTPErrorHandler`; any other return value is serialized via the Binder (JSON by default, negotiated from Accept/Content-Type headers).

### Static files and Proxy

Both `Static` and `Proxy` are **routes** (not global middleware), registered via `srv.Register(wildcardRoute(prefix), handler, methods...)`. The wildcard route pattern is `"prefix/*"`.

- **Static**: Serves the file if it exists on disk; returns `ErrNotFound` (causing the handler to call `c.Next()`) if the file doesn't exist or path traversal is detected. WebSocket requests are skipped. Path traversal is guarded by `withinRoot` using `filepath.Rel`.
- **Proxy**: Matches a URL prefix and forwards to upstream targets (random load balancing via `defaultProxyGetTarget`). Uses `httputil.ReverseProxy` under the hood. `StripPrefix` controls whether the prefix is removed before forwarding (passed via context to avoid races).

### Service / Registry integration

`Server.Service(name)` creates a `registry.Service` that can bulk-register struct methods as routes. The `Handler` attached to a Service acts as the pipeline for all routes in that Service. `Server.Handler(name)` retrieves the `*Handler` for an existing Service.

### Key dependencies

- `github.com/hwcer/cosgo` — provides `registry` (route trie), `binder` (serialization), `scc` (graceful shutdown coordination), `values` (error types), `session`.
- `github.com/hwcer/logger` — structured logging.
- `golang.org/x/crypto` — TLS utilities, ACME/autocert support.

### Context parameter resolution

`Context.Get(key)` searches multiple sources in priority order: Context store → path params → query → body → cookie. This order is configurable via `Server.RequestDataType`. Path params use `registry.Params.Get()` (linear scan, no map allocation). Query and body stores are lazily created on first access.

### Response model

`Response` wraps `http.ResponseWriter` with `written` and `hijacked` flags. `CanWrite()` returns false once any bytes have been written or the connection has been hijacked — `HTTPErrorHandler` and `Handler.write` check this to avoid double-writing. `Write()` supports multiple calls (accumulates output), enabling streaming/`io.Copy` scenarios.

### Security

- **Slowloris protection**: `ReadHeaderTimeout` (20s) and `IdleTimeout` (60s) set by default.
- **Path traversal guard**: `withinRoot()` validates static file paths are within the root directory.
- **Body size limit**: `Server.MaxBodySize` (default 10MB) — reads one extra byte to distinguish "at limit" from "exceeded".
- **Body caching**: `Server.MaxCacheSize` (default 1MB) — bodies ≤ this size are cached on Context for repeated reads.

### Built-in middleware

- `middleware.AccessControlAllow` — CORS handling with origin matching, credentials, and Unity-specific security headers.
- `middleware.AutoCert` — Let's Encrypt automatic certificate management; provides TLS config, HTTP-01 challenge middleware, and HTTP→HTTPS redirect handler.

### TLS utilities

- `TLSConfigAutocert(cacheDir, hosts...)` — creates a `tls.Config` using Let's Encrypt for automatic certificate issuance.
- `TLSConfigParse(certFile, keyFile)` — constructs `tls.Config` from file paths or raw bytes.
- `Listener(network)` — retrieves a registered `MakeListener` (tcp, tcp4, tcp6, http, ws, wss pre-registered; extensible via `RegisterListener`).
