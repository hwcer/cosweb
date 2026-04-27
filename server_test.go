package cosweb

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// newTestServer 返回一个 httptest.Server,包装当前 *Server。
func newTestServer(t *testing.T, s *Server) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return ts
}

// TestMiddlewareNestedSemantics 验证 next() 之后的后置逻辑,在链尾完成后才执行。
func TestMiddlewareNestedSemantics(t *testing.T) {
	s := New()
	var order []string
	s.Use(func(c *Context, next Next) error {
		order = append(order, "m1-before")
		err := next()
		order = append(order, "m1-after")
		return err
	})
	s.Use(func(c *Context, next Next) error {
		order = append(order, "m2-before")
		err := next()
		order = append(order, "m2-after")
		return err
	})
	s.GET("/x", func(c *Context) any {
		order = append(order, "handler")
		return "ok"
	})

	ts := newTestServer(t, s)
	resp, err := http.Get(ts.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	want := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(order) != len(want) {
		t.Fatalf("middleware order mismatch: got %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("at %d: got %q, want %q", i, order[i], want[i])
		}
	}
}

// TestMiddlewareShortCircuit 验证中间件不调 next() 则短路后续中间件和 handler。
func TestMiddlewareShortCircuit(t *testing.T) {
	s := New()
	var handlerRan int32
	s.Use(func(c *Context, next Next) error {
		// 不调用 next,直接写响应
		return c.String("halted")
	})
	s.GET("/x", func(c *Context) any {
		atomic.AddInt32(&handlerRan, 1)
		return "never"
	})

	ts := newTestServer(t, s)
	resp, err := http.Get(ts.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "halted" {
		t.Errorf("expected body 'halted', got %q", body)
	}
	if atomic.LoadInt32(&handlerRan) != 0 {
		t.Errorf("handler should not have run")
	}
}

// TestBodyLimitExceeded 验证超出 MaxBodySize 返回 413。
func TestBodyLimitExceeded(t *testing.T) {
	s := New()
	s.MaxBodySize = 64
	s.POST("/x", func(c *Context) any {
		_, err := c.Buffer()
		if err != nil {
			return err
		}
		return "ok"
	})

	ts := newTestServer(t, s)
	resp, err := http.Post(ts.URL+"/x", "application/octet-stream", strings.NewReader(strings.Repeat("a", 128)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Errorf("expected 413, got %d", resp.StatusCode)
	}
}

// TestBodyLimitAtBoundary 验证恰好等于 MaxBodySize 的 body 不被误判超限。
func TestBodyLimitAtBoundary(t *testing.T) {
	s := New()
	s.MaxBodySize = 16
	s.POST("/x", func(c *Context) any {
		_, err := c.Buffer()
		if err != nil {
			return err
		}
		return "ok"
	})
	ts := newTestServer(t, s)
	resp, err := http.Post(ts.URL+"/x", "application/octet-stream", strings.NewReader(strings.Repeat("a", 16)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 at boundary, got %d", resp.StatusCode)
	}
}

// TestStaticTraversal 验证 ../ 无法越过 root。
func TestStaticTraversal(t *testing.T) {
	root := t.TempDir()
	// 在 root 内写一个可访问文件
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 在 root 同级写一个不该被访问的文件
	parent := filepath.Dir(root)
	secret := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(secret)

	s := New()
	s.Static("/s", root)
	ts := newTestServer(t, s)

	// 合法访问
	resp, err := http.Get(ts.URL + "/s/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "hello" {
		t.Errorf("expected 'hello', got %q", body)
	}

	// 穿越访问: %2E%2E/secret.txt
	resp, err = http.Get(ts.URL + "/s/..%2Fsecret.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), "TOPSECRET") {
		t.Errorf("path traversal succeeded: %q", body)
	}
}

// TestSetGetThroughDefault 验证 c.Set + c.Get 在默认 dataTypes 下互通。
func TestSetGetThroughDefault(t *testing.T) {
	s := New()
	s.Use(func(c *Context, next Next) error {
		c.Set("uid", "u-42")
		return next()
	})
	s.GET("/x", func(c *Context) any {
		// 直接写纯文本,避免 JSON 序列化加引号影响断言
		_ = c.String(c.GetString("uid"))
		return nil
	})
	ts := newTestServer(t, s)
	resp, err := http.Get(ts.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "u-42" {
		t.Errorf("expected 'u-42', got %q", body)
	}
}

// TestResponseMultiWrite 验证 Response.Write 支持多次写入(Stream/io.Copy 场景)。
func TestResponseMultiWrite(t *testing.T) {
	s := New()
	s.GET("/x", func(c *Context) any {
		c.Response.Header().Set(HeaderContentType, "text/plain")
		c.Response.Write([]byte("part1-"))
		c.Response.Write([]byte("part2"))
		return nil
	})
	ts := newTestServer(t, s)
	resp, err := http.Get(ts.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "part1-part2" {
		t.Errorf("expected 'part1-part2', got %q", body)
	}
}
