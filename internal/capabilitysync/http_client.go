package capabilitysync

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"time"
)

func GatewayHTTPClient(certificateAuthorityPEM string) *http.Client {
	if certificateAuthorityPEM == "" {
		return nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(certificateAuthorityPEM)) {
		return nil
	}
	return &http.Client{Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:   true,
		TLSClientConfig:     &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	}}
}
