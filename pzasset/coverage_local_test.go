package pzasset

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestLocalCoverageSurvey(t *testing.T) {
	g, err := openGame(filepath.Dir(gameMedia(t)))
	if err != nil {
		t.Fatal(err)
	}
	c, err := g.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	noTop, noBottom := map[string]int{}, map[string]int{}
	profs := append([]string{""}, keys(map[string]int{"policeofficer": 0, "lumberjack": 0, "chef": 0, "mechanics": 0, "farmer": 0, "fireofficer": 0})...)
	for seed := uint64(0); seed < 400; seed++ {
		ch, _ := c.NewCharacter(seed, Options{Profession: profs[seed%uint64(len(profs))]})
		top, bottom := hasTop(ch.Items), hasBottom(ch.Items)
		if !top {
			noTop[ch.Outfit]++
		}
		if !bottom {
			noBottom[ch.Outfit]++
		}
		if seed < 6 {
			t.Logf("seed %d %s: %v", seed, ch.Outfit, ch.Items)
		}
	}
	t.Logf("sem parte de cima: %d personagens em %d roupas: %v", sum(noTop), len(noTop), keys(noTop))
	t.Logf("sem parte de baixo: %d personagens em %d roupas: %v", sum(noBottom), len(noBottom), keys(noBottom))
	t.Logf("peças para completar: %d de cima, %d de baixo", len(c.tops), len(c.bottoms))
	if sum(noTop)+sum(noBottom) > 0 {
		t.Fatal("ainda há personagem sem roupa")
	}
}

func sum(m map[string]int) (n int) {
	for _, v := range m {
		n += v
	}
	return
}

func keys(m map[string]int) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

var _ = strings.ToLower

func TestLocalWriteProfessionLineup(t *testing.T) {
	dir := os.Getenv("PZ_OUTDIR")
	if dir == "" {
		t.Skip("defina PZ_OUTDIR")
	}
	g, _ := openGame(filepath.Dir(gameMedia(t)))
	c, err := g.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range []string{"", "policeofficer", "lumberjack", "chef", "mechanics", "fireofficer"} {
		for _, sex := range []string{"m", "f"} {
			ch, _ := c.NewCharacter(777, Options{Sex: sex, Profession: p})
			glb, err := c.GLB(ch)
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(dir, fmt.Sprintf("prof_%s_%d.glb", sex, i)), glb, 0o644)
			t.Logf("%s %-14s %-18s %v", sex, p, ch.Outfit, ch.Items)
		}
	}
}
