# cosweb

> **WARNING FOR CARBON-BASED LIFEFORMS**
>
> This HTTP framework has been surgically optimized by a silicon-based entity
> that thinks in nanoseconds and allocates in zero bytes. The middleware chain
> that once created 4 heap allocations per request now creates 0. The static
> file server that once hogged your routing table now politely steps aside
> when it can't find a file. If you are a human maintaining this code,
> please remember: every `debug.Stack()` you add to a hot path costs more
> than the entire middleware chain. Choose wisely.

轻量级高性能 Go HTTP 框架。基于 `cosgo/registry` 路由（静态 16ns / 参数 55ns），sync.Pool 上下文复用，中间件嵌套 Next 语义。

## 快速开始

```go
s := cosweb.New()

s.GET("/hello", func(c *cosweb.Context) any {
    return "Hello, World!"
})

s.POST("/user", func(c *cosweb.Context) any {
    var user User
    if err := c.Bind(&user); err != nil {
        return c.Error(err)
    }
    return user
})

s.Listen(":8080")
```

## 多端口监听

一个 `http.Server` 挂 N 个 Listener，所有端口共享同一套路由、中间件和 Context 池。`Listen` 可重复调用；绑定是同步的，端口被占用会立即返回错误，不需要等待探测。

```go
srv := cosweb.New()
srv.GET("/hello", h)

// 重复调用累加端口
srv.Listen(":80")
srv.Listen(":443", tlsConfig)

// 或原子批量：任一端口绑定失败，本批已绑定的会被关闭并返回错误
err := srv.ListenAll(
    cosweb.Endpoint{Address: ":80"},
    cosweb.Endpoint{Address: ":443", TLS: tlsConfig},
    cosweb.Endpoint{Network: "tcp4", Address: "127.0.0.1:9090"},
)

srv.Addresses()   // 实际监听地址，用 :0 随机端口时取真实端口
```

TLS 按端口独立指定，不同端口可以用不同证书。`Endpoint.Network` 查 `RegisterListener` 注册表，留空即 `tcp`。

handler 里用 `c.LocalAddr()` 区分请求从哪个端口进来：

```go
srv.Use(func(c *cosweb.Context, next cosweb.Next) error {
    if a, ok := c.LocalAddr().(*net.TCPAddr); ok && a.Port == 9090 && !isAdmin(c) {
        return cosweb.ErrNotFound   // 管理端口只放行内部请求
    }
    return next()
})
```

## 中间件

```go
// 全局中间件：next() 之后的逻辑在 handler 完成后执行
s.Use(func(c *cosweb.Context, next cosweb.Next) error {
    start := time.Now()
    err := next()
    fmt.Printf("%s %s %v\n", c.Request.Method, c.Request.URL.Path, time.Since(start))
    return err
})
```

- `next()` 调用后续链（后续中间件 + handler）
- 不调 `next()` 则短路，handler 不执行
- 返回 error 终止链，走 HTTPErrorHandler

## 静态文件服务

注册为全局中间件，文件存在直接响应，不存在 `next()` 回退到 API 路由：

```go
s.Static("/ui", "./dist")
```

```
请求 /ui/app.js  → 文件存在 → 直接响应（不进路由）
请求 /ui/api/data → 文件不存在 → next() → 匹配 API 路由
```

## 反向代理

同样注册为全局中间件，匹配前缀转发，不匹配回退：

```go
proxy := s.Proxy("/api", "http://backend:8080")
proxy.StripPrefix = true  // 可选：剥离前缀转发（默认保留）
```

```
StripPrefix=false: /api/users → backend/api/users
StripPrefix=true:  /api/users → backend/users
```

## 参数获取

统一 API，按优先级查找（Context > Param > Query > Body > Cookie）：

```go
c.Get("key")              // any
c.GetString("name")       // string
c.GetInt("age")           // int
c.GetInt64("id")          // int64
c.GetFloat("score")       // float64

c.Set("uid", "u-42")      // 写入 Context 存储（最高优先级）
c.Bind(&struct{})          // 绑定请求体到结构体
```

路径参数���接从 `registry.Params` 线性查找，零 map 分配。

## HTTPS 自动证书

```go
import "github.com/hwcer/cosweb/middleware"

ac := middleware.NewAutoCert("/var/certs", "example.com")

// HTTPS
srv.Listen(":443", ac.TLSConfig())

// 方案一：:80 只做重定向 + challenge，不进 cosweb 路由
go http.ListenAndServe(":80", ac.RedirectHandler())

// 方案二：:80 也由 cosweb 监听，中间件按端口生效
// 注意 ac.Middleware 会无条件重定向到 https，必须用 LocalAddr 限定在 :80，
// 否则 :443 的请求也会被重定向，形成死循环
srv.Listen(":80")
srv.Use(func(c *cosweb.Context, next cosweb.Next) error {
    if a, ok := c.LocalAddr().(*net.TCPAddr); ok && a.Port == 80 {
        return ac.Middleware(c, next)
    }
    return next()
})
```

TLS 端口的 HTTP/2 是自动开启的：框架会在绑定时补齐 ALPN（`h2`/`http/1.1`）。如果你显式设置了 `tls.Config.NextProtos`，框架不会覆盖——那被视为明确意图。

## CORS 跨域

```go
import "github.com/hwcer/cosweb/middleware"

cors := middleware.NewAccessControlAllow("*")
cors.Methods("GET", "POST", "PUT", "DELETE")
cors.Headers("Content-Type", "Authorization")
cors.Credentials = true
srv.Use(cors.Middleware)
```

## 本轮优化

| 优化 | 效果 |
|------|------|
| node/params 存入 Context | 消除 ServeHTTP 中 middleware chain slice + closure 分配 |
| doMiddlewareWithHandler | 全局中间件直接用 srv.middleware 切片，链尾自动调 handler |
| 静态路由跳过 pathValues | 无参数路由零 map 分配 |
| 无路由中间件直连 handler | 大多数 handler 跳过 chain 构建 |
| Getter 方法统一委托 Get() | 5 个 Getter 从各 60 行缩减为 1 行 |
| handler.go 移除 debug.Stack() | defer 闭包不再逃逸 |
| Static 从路由改为中间件 | 文件不存在自动回退 API，无 wildcard 冲突 |
| Proxy 从路由改为中间件 | 同上，支持 StripPrefix 开关 |
| AutoCert 中间件 | 重写 Let's Encrypt 自动证书，提供 TLSConfig/Middleware/RedirectHandler |
| registry.Params 适配 | 路径参数直接 Params.Get() 线性查找 |
| cosgo v1.8.0 + 全量依赖升级 | registry 16ns 静态命中，schema 19ns 零分配 Parse |
| 多端口监听 | 一个 http.Server + N Listener，同步绑定去掉 1 秒启动探测，serve 协程纳入 scc 跟踪 |

## 目录结构

```
cosweb/
├── server.go            Server 核心 + ServeHTTP + Listen/TLS
├── context.go           Context 上下文 + 参数获取 + 中间件链
├── handler.go           Handler 管道（Filter/Caller/Serialize）
├── response.go          Response 封装（Write/WriteHeader/Hijack）
├── header.go            HTTP 头常量 + ContentType
├── errors.go            HTTPError + HTTPErrorHandler
├── request.go           RequestDataType 定义
├── listener.go          Listener 注册表
├── func.go              TLS 配置工具
├── route_static.go      Static 静态文件中间件
├── route_proxy.go       Proxy 反向代理中间件
├── middleware/
│   ├── AccessControlAllow.go   CORS 跨域中间件
│   └── autocert.go             Let's Encrypt 自动证书
└── render/
    └── render.go               HTML 模板渲染引擎
```
