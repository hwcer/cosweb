package cosweb

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"
)

// newListenServer 返回一个注册了 /hello 的 Server,并在测试结束时关闭全部监听端口。
func newListenServer(t *testing.T) *Server {
	t.Helper()
	s := New()
	s.GET("/hello", func(c *Context) any {
		return "hello"
	})
	t.Cleanup(s.shutdown)
	return s
}

// newListenClient 返回一个禁用长连接的客户端,避免 Shutdown 等待空闲连接。
func newListenClient() *http.Client {
	return &http.Client{
		Timeout:   3 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
}

// getBody 请求 url 并返回状态码与响应体。
func getBody(t *testing.T, client *http.Client, url string) (int, string) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return resp.StatusCode, string(b)
}

// TestListenMultiplePorts 验证多个端口共享同一套路由。
func TestListenMultiplePorts(t *testing.T) {
	s := newListenServer(t)
	if err := s.ListenAll(
		Endpoint{Address: "127.0.0.1:0"},
		Endpoint{Address: "127.0.0.1:0"},
	); err != nil {
		t.Fatalf("ListenAll: %v", err)
	}
	addrs := s.Addresses()
	if len(addrs) != 2 {
		t.Fatalf("Addresses len = %d, want 2", len(addrs))
	}
	if addrs[0].String() == addrs[1].String() {
		t.Fatalf("两个端点拿到了同一个地址: %v", addrs[0])
	}
	client := newListenClient()
	for _, addr := range addrs {
		code, body := getBody(t, client, "http://"+addr.String()+"/hello")
		if code != http.StatusOK {
			t.Fatalf("%v: code=%d body=%s", addr, code, body)
		}
		if body != strconvQuote("hello") {
			t.Fatalf("%v: body=%s", addr, body)
		}
	}
}

// TestListenRepeatable 验证 Listen 可重复调用累加端口,而不是互相覆盖。
func TestListenRepeatable(t *testing.T) {
	s := newListenServer(t)
	if err := s.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("first Listen: %v", err)
	}
	if err := s.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("second Listen: %v", err)
	}
	addrs := s.Addresses()
	if len(addrs) != 2 {
		t.Fatalf("Addresses len = %d, want 2", len(addrs))
	}
	client := newListenClient()
	for _, addr := range addrs {
		if code, _ := getBody(t, client, "http://"+addr.String()+"/hello"); code != http.StatusOK {
			t.Fatalf("%v: code=%d", addr, code)
		}
	}
}

// TestListenAllRollback 验证批量绑定中任一端口失败时,本批已绑定的端口会被关闭。
func TestListenAllRollback(t *testing.T) {
	// 占住一个端口,让后续绑定必然失败
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy: %v", err)
	}
	defer occupied.Close()

	// 先探一个空闲地址,立即释放,用于验证回滚后能重新绑定
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	free := probe.Addr().String()
	_ = probe.Close()

	s := newListenServer(t)
	err = s.ListenAll(
		Endpoint{Address: free},
		Endpoint{Address: occupied.Addr().String()},
	)
	if err == nil {
		t.Fatal("ListenAll 应当因端口被占用而失败")
	}
	if n := len(s.Addresses()); n != 0 {
		t.Fatalf("失败后不应有已启动的监听器,实际 %d 个", n)
	}
	// 回滚生效的判据:第一个端口能被重新绑定
	again, err := net.Listen("tcp", free)
	if err != nil {
		t.Fatalf("回滚失败,%s 仍被占用: %v", free, err)
	}
	_ = again.Close()
}

// TestListenNoStartupDelay 验证绑定是同步的,不再有 1 秒探测延迟。
func TestListenNoStartupDelay(t *testing.T) {
	s := newListenServer(t)
	start := time.Now()
	if err := s.ListenAll(
		Endpoint{Address: "127.0.0.1:0"},
		Endpoint{Address: "127.0.0.1:0"},
	); err != nil {
		t.Fatalf("ListenAll: %v", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("绑定耗时 %v,应当是同步立即返回", d)
	}
}

// TestListenUnknownNetwork 验证未注册的网络类型直接报错。
func TestListenUnknownNetwork(t *testing.T) {
	s := newListenServer(t)
	if err := s.ListenAll(Endpoint{Network: "carrier-pigeon", Address: "127.0.0.1:0"}); err == nil {
		t.Fatal("未注册的 network 应当返回错误")
	}
}

// TestContextLocalAddr 验证 handler 能通过 LocalAddr 区分请求到达的端口。
func TestContextLocalAddr(t *testing.T) {
	s := New()
	s.GET("/where", func(c *Context) any {
		addr := c.LocalAddr()
		if addr == nil {
			return "nil"
		}
		return addr.String()
	})
	t.Cleanup(s.shutdown)
	if err := s.ListenAll(
		Endpoint{Address: "127.0.0.1:0"},
		Endpoint{Address: "127.0.0.1:0"},
	); err != nil {
		t.Fatalf("ListenAll: %v", err)
	}
	client := newListenClient()
	for _, addr := range s.Addresses() {
		_, body := getBody(t, client, "http://"+addr.String()+"/where")
		if body != strconvQuote(addr.String()) {
			t.Fatalf("LocalAddr = %s, want %s", body, addr.String())
		}
	}
}

// TestShutdownClosesAllPorts 验证 shutdown 关闭全部端口,而不只是第一个。
func TestShutdownClosesAllPorts(t *testing.T) {
	s := New()
	s.GET("/hello", func(c *Context) any { return "hello" })
	if err := s.ListenAll(
		Endpoint{Address: "127.0.0.1:0"},
		Endpoint{Address: "127.0.0.1:0"},
	); err != nil {
		t.Fatalf("ListenAll: %v", err)
	}
	addrs := s.Addresses()
	client := newListenClient()
	for _, addr := range addrs {
		if code, _ := getBody(t, client, "http://"+addr.String()+"/hello"); code != http.StatusOK {
			t.Fatalf("%v 关闭前应可访问", addr)
		}
	}
	s.shutdown()
	for _, addr := range addrs {
		if _, err := client.Get("http://" + addr.String() + "/hello"); err == nil {
			t.Fatalf("%v 关闭后仍可访问", addr)
		}
	}
}

// TestListenTLSHTTP2 验证 TLS 端口仍然协商出 HTTP/2。
// 手工包装 TLS listener 后 Serve 只在 srv.Server.TLSConfig 为 nil 时自动装配 h2,
// 且 ALPN 需要 withALPN 补齐,这个用例同时守住这两点。
func TestListenTLSHTTP2(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	cfg, err := TLSConfigParse(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("TLSConfigParse: %v", err)
	}
	if len(cfg.NextProtos) != 0 {
		t.Fatalf("前置条件变了:TLSConfigParse 现在会设置 NextProtos = %v", cfg.NextProtos)
	}

	s := newListenServer(t)
	if err = s.ListenAll(Endpoint{Address: "127.0.0.1:0", TLS: cfg}); err != nil {
		t.Fatalf("ListenAll: %v", err)
	}
	if s.Server.TLSConfig != nil {
		t.Fatal("srv.Server.TLSConfig 必须保持 nil,否则 Serve 不会自动装配 HTTP/2")
	}
	// withALPN 不得污染调用方传入的 config
	if len(cfg.NextProtos) != 0 {
		t.Fatalf("withALPN 污染了调用方的 tls.Config: %v", cfg.NextProtos)
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			ForceAttemptHTTP2: true,
		},
	}
	defer client.CloseIdleConnections()

	addr := s.Addresses()[0].String()
	resp, err := client.Get("https://" + addr + "/hello")
	if err != nil {
		t.Fatalf("https get: %v", err)
	}
	defer resp.Body.Close()
	if resp.Proto != "HTTP/2.0" {
		t.Fatalf("Proto = %s, want HTTP/2.0", resp.Proto)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("code = %d", resp.StatusCode)
	}
}

// TestWithALPNRespectsCaller 验证调用方显式指定 ALPN 时不被覆盖。
func TestWithALPNRespectsCaller(t *testing.T) {
	cfg := &tls.Config{NextProtos: []string{"http/1.1"}}
	if got := withALPN(cfg); got != cfg {
		t.Fatal("NextProtos 非空时应原样返回")
	}
	if withALPN(nil) != nil {
		t.Fatal("nil 应返回 nil")
	}
	empty := &tls.Config{}
	got := withALPN(empty)
	if got == empty {
		t.Fatal("应当返回克隆而非原对象")
	}
	if len(got.NextProtos) != 2 || got.NextProtos[0] != "h2" {
		t.Fatalf("NextProtos = %v", got.NextProtos)
	}
	if len(empty.NextProtos) != 0 {
		t.Fatalf("原对象被污染: %v", empty.NextProtos)
	}
}

// strconvQuote 按 JSON 序列化后的形式给字符串加引号,便于比对响应体。
func strconvQuote(s string) string {
	return "\"" + s + "\""
}

// selfSignedPEM 生成一张 127.0.0.1 的自签证书。
func selfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "cosweb-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}
