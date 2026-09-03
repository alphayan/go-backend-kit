// Test-only loopback proxy. Uses httptest's certificate; never deploy this tool.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("E2E_DISPOSABLE_DATABASE") != "1" {
		return fmt.Errorf("TLS fixture requires E2E_DISPOSABLE_DATABASE=1")
	}
	port := 4187
	if value := os.Getenv("E2E_PORT"); value != "" {
		var err error
		port, err = strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid E2E_PORT")
		}
	}
	if port < 1024 || port > 65533 {
		return fmt.Errorf("E2E_PORT must be between 1024 and 65533")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
	proxy := newProxy(target)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 5 * time.Second
	proxy.Transport = transport
	defer transport.CloseIdleConnections()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.Host)
		if host == "attacker.invalid.test" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<!doctype html><title>Cross-site test origin</title>"))
			return
		}
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		// Observe actual browser headers at the network boundary. Expose only
		// metadata, never cookie values, in this test-only fixture.
		w.Header().Set("X-E2E-Fetch-Site", r.Header.Get("Sec-Fetch-Site"))
		w.Header().Set("X-E2E-Cookie-Present", strconv.FormatBool(r.Header.Get("Cookie") != ""))
		proxy.ServeHTTP(w, r)
	})
	tlsServer, err := listen(port+1, handler, true)
	if err != nil {
		return err
	}
	defer tlsServer.Close()
	// Deliberately insecure comparison endpoint: proves Secure cookies are rejected
	// on HTTP on a non-localhost hostname. It must not be a production redirect.
	plainServer, err := listen(port+2, handler, false)
	if err != nil {
		return err
	}
	defer plainServer.Close()
	<-ctx.Done()
	return nil
}

func listen(port int, handler http.Handler, secure bool) (*httptest.Server, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	server := &httptest.Server{Listener: listener, Config: &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second,
	}}
	if secure {
		server.StartTLS()
	} else {
		server.Start()
	}
	return server, nil
}

func newProxy(target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		// Rewrite already strips Forwarded and X-Forwarded-{For,Host,Proto}.
		// Drop alternate Echo scheme/IP headers too; preserve Origin/Fetch metadata.
		for _, name := range []string{"X-Real-IP", "X-Forwarded-Protocol", "X-Forwarded-Ssl", "X-Url-Scheme"} {
			r.Out.Header.Del(name)
		}
		r.SetURL(target)
		r.Out.Host = r.In.Host
		r.SetXForwarded()
	}}
}
