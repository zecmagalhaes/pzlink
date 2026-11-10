package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

//go:embed web
var webFS embed.FS

// Everything the page loads comes from this program: no external scripts,
// styles, images or connections. Embedded 3D models use blob: URLs.
const contentSecurityPolicy = "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'unsafe-inline'; " +
	"connect-src 'self' blob: data:; img-src 'self' data: blob:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// routes serves the UI only to requests addressed exactly to 127.0.0.1:port,
// which blocks DNS-rebinding pages from reading the state (and the invite key).
func (a *app) routes(port int) http.Handler {
	mux := http.NewServeMux()
	allowedHost := fmt.Sprintf("127.0.0.1:%d", port)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != allowedHost {
			http.Error(w, "acesso negado", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if !strings.HasPrefix(r.URL.Path, "/pz/") && !strings.HasPrefix(r.URL.Path, "/static/") {
			h.Set("Cache-Control", "no-store")
		}
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		mux.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		page, _ := webFS.ReadFile("web/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	static, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /api/lobby", func(w http.ResponseWriter, r *http.Request) {
		st := a.lobby()
		if n, err := strconv.Atoi(r.URL.Query().Get("demo")); err == nil {
			st = a.withDemo(st, n)
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("GET /pz/char/{seed}", func(w http.ResponseWriter, r *http.Request) {
		seed, err := strconv.ParseUint(r.PathValue("seed"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		glb, err := a.assets.characterGLB(seed)
		if err != nil {
			http.Error(w, "personagem indisponível", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "model/gltf-binary")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.Write(glb)
	})
	mux.HandleFunc("GET /pz/prop/{id}/{part}", a.propHandler)
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.snapshot())
	})
	mux.HandleFunc("POST /api/host", a.guard(func(body map[string]string) error {
		return a.startHost(body["key"], body["guestKey"], body["nick"])
	}))
	mux.HandleFunc("POST /api/join", a.guard(func(body map[string]string) error {
		return a.startJoin(body["invite"], body["nick"])
	}))
	mux.HandleFunc("POST /api/nick", a.guard(func(body map[string]string) error {
		n := sanitizeName(body["nick"])
		if n == "" {
			return nil
		}
		a.mu.Lock()
		running := a.mode != ""
		if !running {
			a.cfg.Nick = n
			a.saveConfig()
		}
		a.mu.Unlock()
		return nil
	}))
	mux.HandleFunc("POST /api/stop", a.guard(func(map[string]string) error {
		a.stop()
		return nil
	}))
	mux.HandleFunc("POST /api/forget-key", a.guard(func(map[string]string) error {
		a.mu.Lock()
		a.cfg.AuthKey, a.cfg.GuestKey = "", ""
		a.saveConfig()
		a.mu.Unlock()
		return nil
	}))
	return handler
}

// guard parses a JSON body and rejects requests from other sites (only the
// local page may drive the app).
func (a *app) guard(fn func(map[string]string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && !strings.HasPrefix(o, "http://127.0.0.1:") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "origem não permitida"})
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "json esperado"})
			return
		}
		body := map[string]string{}
		json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)
		if err := fn(body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
