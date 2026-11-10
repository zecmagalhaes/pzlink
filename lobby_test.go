package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A hostile host could send anything in the roster; the guest must bound it.
func TestCleanRoster(t *testing.T) {
	var in []rosterEntry
	for i := 0; i < 40; i++ {
		in = append(in, rosterEntry{Name: "jogador", PingMs: 50, Level: "good"})
	}
	in[0] = rosterEntry{Name: "<script>alert(1)</script>", Level: "hackeado", PingMs: -5, Path: strings.Repeat("x", 500)}
	in[1] = rosterEntry{Name: "   ", Level: "good"}
	out := cleanRoster(in)
	if len(out) > 16 {
		t.Fatalf("lista sem limite: %d", len(out))
	}
	first := out[0]
	if strings.ContainsAny(first.Name, "<>()") || first.Level != "wait" || first.PingMs != 0 || len(first.Path) > 40 {
		t.Fatalf("entrada hostil passou: %+v", first)
	}
	for _, e := range out {
		if e.Name == "" {
			t.Fatal("nome vazio passou")
		}
	}
}

func TestSeedStableAndCaseInsensitive(t *testing.T) {
	if seedFor("Rafa") != seedFor(" rafa ") || seedFor("rafa") == seedFor("duda") {
		t.Fatal("semente do personagem instável")
	}
	if displayName("joao-a3") != "joao" || displayName("ze-do-mato") != "ze-do-mato" {
		t.Fatal("apelido do aparelho mal convertido")
	}
}

func TestLobbyRoutesWithoutGame(t *testing.T) {
	t.Setenv("PZ_HOME", t.TempDir()) // pasta sem o jogo
	a := newApp(t.TempDir())
	h := a.routes(7468)
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:7468"+path, nil)
		r.Host = "127.0.0.1:7468"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := get("/api/lobby")
	var st lobbyState
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &st) != nil || st.GameFound || len(st.Players) != 1 {
		t.Fatalf("lobby sem jogo: %d %s", w.Code, w.Body)
	}
	for path, want := range map[string]int{
		"/pz/char/123":                         http.StatusNotFound, // sem jogo
		"/pz/char/abc":                         http.StatusNotFound, // semente inválida
		"/pz/prop/barraca/model":               http.StatusNotFound, // sem jogo
		"/static/lobby.js":                     http.StatusOK,
		"/static/vendor/three/three.module.js": http.StatusOK,
	} {
		if c := get(path).Code; c != want {
			t.Errorf("%s: veio %d, esperava %d", path, c, want)
		}
	}
	if c := get("/pz/prop/../../x/model").Code; c == http.StatusOK {
		t.Error("caminho com ../ não pode responder com sucesso")
	}
	csp := get("/").Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "http") {
		t.Fatalf("CSP inesperada: %s", csp)
	}
}

func TestCleanProfile(t *testing.T) {
	cases := map[profile]profile{
		{Sex: "m", Profession: "PoliceOfficer"}: {Sex: "m", Profession: "policeofficer"},
		{Sex: "x", Profession: "doctor"}:        {Sex: "r", Profession: "doctor"},
		{Sex: "f", Profession: "../etc/passwd"}: {Sex: "f", Profession: ""},
		{Sex: "f", Profession: "<b>hacker</b>"}: {Sex: "f", Profession: ""},
		{Profession: strings.Repeat("a", 64)}:   {Sex: "r", Profession: ""},
	}
	for in, want := range cases {
		if got := cleanProfile(in); got != want {
			t.Errorf("%+v: veio %+v, esperava %+v", in, got, want)
		}
	}
}

// A guest can only set the profile of its own tailnet address, and the host
// remembers at most 64 devices.
func TestSetProfileBounded(t *testing.T) {
	a := newApp(t.TempDir())
	for i := 0; i < 100; i++ {
		a.setProfile(fmt.Sprintf("100.100.0.%d", i), profile{Sex: "f", Profession: "doctor"})
	}
	if len(a.profiles) != 64 {
		t.Fatalf("perfis sem limite: %d", len(a.profiles))
	}
	a.setProfile("100.100.0.1", profile{Sex: "m", Profession: "chef"}) // already known: updates
	if a.profiles["100.100.0.1"].Profession != "chef" {
		t.Fatal("perfil conhecido não foi atualizado")
	}
}

func TestProfileEndpoint(t *testing.T) {
	t.Setenv("PZ_HOME", t.TempDir())
	a := newApp(t.TempDir())
	h := a.routes(7468)
	r := httptest.NewRequest("POST", "http://127.0.0.1:7468/api/profile",
		strings.NewReader(`{"nick":"Marina","sex":"f","profession":"policeofficer"}`))
	r.Host = "127.0.0.1:7468"
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("perfil recusado: %d %s", w.Code, w.Body)
	}
	if a.cfg.Nick != "marina" || a.cfg.Sex != "f" || a.cfg.Profession != "policeofficer" {
		t.Fatalf("perfil não salvo: %+v", a.cfg)
	}
	st := a.lobby()
	if len(st.Players) != 1 || st.Players[0].Online || st.Players[0].Name != "marina" {
		t.Fatalf("lobby sem conexão inesperado: %+v", st.Players)
	}
}
