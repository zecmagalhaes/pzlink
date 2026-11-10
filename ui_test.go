package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIHostCheck(t *testing.T) {
	a := newApp(t.TempDir())
	h := a.routes(7468)

	cases := []struct {
		host, origin, method, path string
		want                       int
	}{
		{"127.0.0.1:7468", "", "GET", "/api/state", 200},
		{"127.0.0.1:7468", "", "GET", "/", 200},
		{"evil.example:7468", "", "GET", "/api/state", 403}, // DNS rebinding
		{"localhost:7468", "", "GET", "/api/state", 403},
		{"127.0.0.1:9999", "", "GET", "/api/state", 403},
		{"127.0.0.1:7468", "http://evil.example", "POST", "/api/stop", 403},
		{"127.0.0.1:7468", "http://127.0.0.1:7468", "POST", "/api/stop", 200},
	}
	for _, c := range cases {
		var body *strings.Reader = strings.NewReader("{}")
		r := httptest.NewRequest(c.method, "http://"+c.host+c.path, body)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s %s host=%s origin=%s: veio %d, esperava %d", c.method, c.path, c.host, c.origin, w.Code, c.want)
		}
	}
}
