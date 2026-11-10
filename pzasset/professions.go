package pzasset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Profession is a starting occupation as the game defines it
// (media/scripts/generated/characters/character_professions.txt).
type Profession struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Cost   int      `json:"cost"`
	Boosts []Boost  `json:"boosts"`
	Traits []string `json:"traits,omitempty"`
}

type Boost struct {
	Perk  string `json:"perk"`
	Name  string `json:"name"`
	Level int    `json:"level"`
}

// profession -> outfits from clothing.xml that fit it (first one found wins
// the draw only among those that exist for the character's sex)
var professionOutfits = map[string][]string{
	"burglar":            {"BankRobber", "BankRobberSuit", "Thug", "Punk"},
	"burgerflipper":      {"Cook_Spiffos", "Cook_Jays", "Waiter_Spiffo"},
	"carpenter":          {"Foreman", "Woodcut", "Redneck", "Trucker"},
	"chef":               {"Chef"},
	"constructionworker": {"ConstructionWorker", "Trucker"},
	"doctor":             {"Doctor"},
	"electrician":        {"Foreman", "Fossoil"},
	"engineer":           {"OfficeWorker", "OfficeWorkerSkirt", "IT", "Foreman"},
	"farmer":             {"Farmer"},
	"fireofficer":        {"Fireman"},
	"fisherman":          {"Fisherman", "Camper", "Hobbyist"},
	"fitnessinstructor":  {"FitnessInstructor"},
	"lumberjack":         {"Woodcut", "Redneck", "Camper"},
	"mechanics":          {"Mechanic", "Fossoil", "Gas2Go"},
	"metalworker":        {"MetalWorker", "Fossoil", "Trucker"},
	"nurse":              {"Nurse"},
	"parkranger":         {"Ranger"},
	"policeofficer":      {"Police"},
	"rancher":            {"Farmer", "Redneck"},
	"repairman":          {"Mechanic", "Foreman", "Fossoil", "Gas2Go"},
	"securityguard":      {"Security", "AirportSecurityTarmac"},
	"smither":            {"MetalWorker", "Fossoil", "Redneck"},
	"tailor":             {"Classy", "OfficeWorker"},
	"veteran":            {"Veteran", "ArmyCamoGreen"},
}

// names the game translates in a way that reads oddly in the lobby
var professionNameOverride = map[string]string{"unemployed": "Desempregado"}

var (
	defStartRe = regexp.MustCompile(`(?m)^\s*character_(profession|trait)_definition\s+base:(\w+)\s*\{`)
	kvRe       = regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*(.*?),?\s*$`)
)

// scriptDefs reads "character_X_definition base:id { Key = Value, ... }".
func scriptDefs(src, kind string) map[string]map[string]string {
	out := map[string]map[string]string{}
	locs := defStartRe.FindAllStringSubmatchIndex(src, -1)
	for _, l := range locs {
		if src[l[2]:l[3]] != kind {
			continue
		}
		id := src[l[4]:l[5]]
		end := strings.Index(src[l[1]:], "}")
		if end < 0 {
			continue
		}
		fields := map[string]string{}
		for _, m := range kvRe.FindAllStringSubmatch(src[l[1]:l[1]+end], -1) {
			fields[m[1]] = strings.TrimSpace(m[2])
		}
		out[id] = fields
	}
	return out
}

// translations loads the game's UI strings, Brazilian Portuguese first.
func (g *Game) translations() map[string]string {
	tr := map[string]string{}
	for _, lang := range []string{"EN", "PTBR"} { // PTBR overrides EN
		for _, f := range []string{"UI.json", "IG_UI.json"} {
			b, err := os.ReadFile(filepath.Join(g.Root, "media", "lua", "shared", "Translate", lang, f))
			if err != nil {
				continue
			}
			var m map[string]any
			if json.Unmarshal(b, &m) != nil {
				continue
			}
			for k, v := range m {
				if s, ok := v.(string); ok && s != "" {
					tr[strings.ToLower(k)] = s
				}
			}
		}
	}
	return tr
}

// Professions lists the starting professions with translated names.
func (g *Game) Professions() []Profession {
	dir := filepath.Join(g.Root, "media", "scripts", "generated", "characters")
	pb, err := os.ReadFile(filepath.Join(dir, "character_professions.txt"))
	if err != nil {
		return nil
	}
	tb, _ := os.ReadFile(filepath.Join(dir, "character_traits.txt"))
	profs := scriptDefs(string(pb), "profession")
	traits := scriptDefs(string(tb), "trait")
	tr := g.translations()
	name := func(key, fallback string) string {
		if s, ok := tr[strings.ToLower(key)]; ok {
			return s
		}
		return fallback
	}

	var out []Profession
	for id, f := range profs {
		p := Profession{ID: id, Name: name(f["UIName"], id)}
		if n, ok := professionNameOverride[id]; ok {
			p.Name = n
		}
		p.Cost, _ = strconv.Atoi(f["Cost"])
		for _, b := range strings.Split(f["XPBoosts"], ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(b), "=")
			if !ok {
				continue
			}
			lvl, err := strconv.Atoi(v)
			if err != nil || lvl <= 0 {
				continue
			}
			p.Boosts = append(p.Boosts, Boost{Perk: k, Name: name("IGUI_perks_"+k, k), Level: min(lvl, 10)})
		}
		sort.Slice(p.Boosts, func(i, j int) bool {
			if p.Boosts[i].Level != p.Boosts[j].Level {
				return p.Boosts[i].Level > p.Boosts[j].Level
			}
			return p.Boosts[i].Name < p.Boosts[j].Name
		})
		for _, t := range strings.Split(f["GrantedTraits"], ";") {
			t = strings.TrimPrefix(strings.TrimSpace(t), "base:")
			if t == "" || t == id {
				continue // the profession's own trait just repeats its name
			}
			if td, ok := traits[t]; ok {
				p.Traits = append(p.Traits, name(td["UIName"], t))
			}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].ID == "unemployed") != (out[j].ID == "unemployed") {
			return out[i].ID == "unemployed"
		}
		return out[i].Name < out[j].Name
	})
	return out
}
