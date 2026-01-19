# cosweb

cosweb 是一个轻量级、高性能的 Go HTTP 服务器框架，专为构建 API 服务和 Web 应用而设计。

## 特性

- **高性能**：基于 Go 语言的并发特性，提供高性能的 HTTP 服务
- **轻量级**：核心代码简洁，依赖少，易于理解和扩展
- **路由系统**：支持 RESTful API 路由，支持 HTTP 方法匹配
- **中间件**：支持全局和路由级中间件
- **参数解析**：自动解析 URL 查询参数、路径参数、表单数据和 JSON 数据
- **统一存储**：使用 `values.Values` 统一存储和访问不同来源的参数
- **请求体限制**：支持配置最大请求体大小和缓存大小
- **静态文件服务**：内置静态文件服务功能
- **代理功能**：支持 HTTP 代理

## 快速开始

### 安装

```bash
go get github.com/hwcer/cosweb
```

### 基本用法

```go
package main

import (
    "github.com/hwcer/cosweb"
)

func main() {
    // 创建服务器实例
    s := cosweb.New()
    
    // 注册路由
    s.GET("/hello", func(c *cosweb.Context) any {
        return "Hello, World!"
    })
    
    s.POST("/user", func(c *cosweb.Context) any {
        name := c.GetString("name")
        return map[string]string{
            "message": "Hello, " + name,
        }
    })
    
    // 启动服务器
    s.Listen(":8080")
}
```

### 中间件

```go
package main

import (
    "github.com/hwcer/cosweb"
)

func main() {
    s := cosweb.New()
    
    // 注册全局中间件
    s.Use(func(c *cosweb.Context) error {
        // 执行前逻辑
        c.Set("start", time.Now())
        
        // 继续处理请求
        err := c.Next()
        
        // 执行后逻辑
        duration := time.Since(c.Get("start").(time.Time))
        fmt.Printf("Request took %v\n", duration)
        
        return err
    })
    
    s.GET("/hello", func(c *cosweb.Context) any {
        return "Hello, World!"
    })
    
    s.Listen(":8080")
}
```

## 核心功能

### 路由系统

cosweb 使用 `registry` 包管理路由，支持多种 HTTP 方法和路径匹配：

```go
// 注册 GET 路由
s.GET("/api/users", getUsers)

// 注册 POST 路由
s.POST("/api/users", createUser)

// 注册 PUT 路由
s.PUT("/api/users/:id", updateUser)

// 注册 DELETE 路由
s.DELETE("/api/users/:id", deleteUser)
```

### 参数获取

cosweb 提供了统一的参数获取接口，支持从多种来源获取参数：

```go
// 获取字符串参数
name := c.GetString("name")

// 获取整数参数
age := c.GetInt("age")

// 获取浮点数参数
score := c.GetFloat("score")

// 获取路径参数
id := c.GetString("id")
```

### 绑定数据

cosweb 支持自动绑定请求数据到结构体：

```go
type User struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

func createUser(c *cosweb.Context) any {
    var user User
    if err := c.Bind(&user); err != nil {
        return c.Error(err)
    }
    // 处理用户数据
    return user
}
```

### 静态文件服务

cosweb 内置静态文件服务功能：

```go
// 注册静态文件服务
s.Static("/static", "./public")
```

### 代理功能

cosweb 支持 HTTP 代理：

```go
// 注册代理
proxy := s.Proxy("/api", "http://api.example.com")
```

## API 参考

### Server 结构体

```go
type Server struct {
    // 内部字段
}
```

#### 方法

- `New() *Server` - 创建新的服务器实例
- `Use(middleware MiddlewareFunc)` - 注册全局中间件
- `GET(path string, handler func(*Context) any)` - 注册 GET 路由
- `POST(path string, handler func(*Context) any)` - 注册 POST 路由
- `PUT(path string, handler func(*Context) any)` - 注册 PUT 路由
- `DELETE(path string, handler func(*Context) any)` - 注册 DELETE 路由
- `Static(prefix, root string, method ...string)` - 注册静态文件服务
- `Proxy(prefix, address string, method ...string) *Proxy` - 注册 HTTP 代理
- `Listen(address string, tlsConfig ...*tls.Config) error` - 启动 HTTP 服务器
- `TLS(address any, certFile, keyFile string) error` - 启动 HTTPS 服务器

### Context 结构体

```go
type Context struct {
    // 内部字段
}
```

#### 方法

- `Get(key string, dataTypes ...RequestDataType) interface{}` - 获取参数
- `GetString(key string, dataTypes ...RequestDataType) string` - 获取字符串参数
- `GetInt(key string, dataTypes ...RequestDataType) int` - 获取整数参数
- `GetInt64(key string, dataTypes ...RequestDataType) int64` - 获取 int64 参数
- `GetFloat(key string, dataTypes ...RequestDataType) float64` - 获取浮点数参数
- `Set(key string, val any)` - 设置上下文参数
- `Bind(i any) error` - 绑定请求数据到结构体
- `Buffer() (*bytes.Buffer, error)` - 获取请求体缓冲区
- `Error(format any) error` - 创建错误
- `Errorf(code int32, format any, args ...any) error` - 创建带错误码的错误

## 性能优化

### 内存池

cosweb 使用 `sync.Pool` 管理 `Context` 对象，减少内存分配和垃圾回收开销。

### 参数存储

使用 `values.Values` 统一存储和访问不同来源的参数，减少类型转换和内存分配。

### 请求体限制

通过配置 `MaxBodySize` 和 `MaxCacheSize`，限制请求体大小和缓存大小，防止内存溢出。

### 路由优化

使用高效的路由匹配算法，减少路由查找时间。

## 配置选项

### 服务器配置

```go
s := cosweb.New()

// 设置最大请求体大小（默认 10MB）
s.MaxBodySize = 10 << 20 // 10 MB

// 设置最大缓存大小（默认 1MB）
s.MaxCacheSize = 1 << 20 // 1 MB

// 设置默认请求数据类型
s.RequestDataType = cosweb.RequestDataTypeMap{
    cosweb.RequestDataTypeParam,
    cosweb.RequestDataTypeQuery,
    cosweb.RequestDataTypeBody,
}
```

## 贡献指南

1. Fork 仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开 Pull Request

## 许可证

cosweb 采用 MIT 许可证，详见 [LICENSE](LICENSE) 文件。