package cosweb

import (
	"crypto/tls"
	"os"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// TLSConfigAutocert 使用 Let's Encrypt 自动申请证书。
// cacheDir 为证书本地缓存目录(为空则不缓存,不建议生产环境使用);
// hosts 为允许签发的域名白名单(为空则不限制,生产环境不建议)。
func TLSConfigAutocert(cacheDir string, hosts ...string) (c *tls.Config, err error) {
	m := &autocert.Manager{Prompt: autocert.AcceptTOS}
	if cacheDir != "" {
		m.Cache = autocert.DirCache(cacheDir)
	}
	if len(hosts) > 0 {
		m.HostPolicy = autocert.HostWhitelist(hosts...)
	}
	c = new(tls.Config)
	c.GetCertificate = m.GetCertificate
	c.NextProtos = append(c.NextProtos, acme.ALPNProto)
	return
}

func filepathOrContent(fileOrContent any) (content []byte, err error) {
	switch v := fileOrContent.(type) {
	case string:
		return os.ReadFile(v)
	case []byte:
		return v, nil
	default:
		return nil, ErrInvalidCertOrKeyType
	}
}

// TLSConfigParse 通过文件路径或原始字节构造 tls.Config。
func TLSConfigParse(certFile, keyFile any) (cfg *tls.Config, err error) {
	var cert []byte
	if cert, err = filepathOrContent(certFile); err != nil {
		return
	}
	var key []byte
	if key, err = filepathOrContent(keyFile); err != nil {
		return
	}
	cfg = new(tls.Config)
	cfg.Certificates = make([]tls.Certificate, 1)
	if cfg.Certificates[0], err = tls.X509KeyPair(cert, key); err != nil {
		return
	}
	return
}
