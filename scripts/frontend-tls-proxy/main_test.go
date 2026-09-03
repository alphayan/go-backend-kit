package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestProxyPreservesOriginAndReplacesForwardedHeaders(t *testing.T) {
	observed := make(chan *http.Request, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "https://app.example.test:4188/auth/login", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-IP", "X-Forwarded-Protocol", "X-Forwarded-Ssl", "X-Url-Scheme"} {
		request.Header.Set(header, "forged")
	}
	request.Header.Set("Origin", "https://attacker.invalid.test:4188")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	request.Header.Set("Cookie", "synthetic=value")
	response := httptest.NewRecorder()
	newProxy(target).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
	got := <-observed
	if got.Host != request.Host {
		t.Fatalf("Host=%q, want %q", got.Host, request.Host)
	}
	for name, want := range map[string]string{
		"X-Forwarded-For": "192.0.2.1", "X-Forwarded-Host": request.Host, "X-Forwarded-Proto": "https",
		"Origin": request.Header.Get("Origin"), "Sec-Fetch-Site": "cross-site",
		"Cookie":    "synthetic=value",
		"Forwarded": "", "X-Real-IP": "", "X-Forwarded-Protocol": "", "X-Forwarded-Ssl": "", "X-Url-Scheme": "",
	} {
		if value := got.Header.Get(name); value != want {
			t.Errorf("%s=%q, want %q", name, value, want)
		}
	}
}
