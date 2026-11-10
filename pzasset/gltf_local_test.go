package pzasset

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestLocalWriteSample writes a GLB for visual checks when PZ_OUT is set.
func TestLocalWriteSample(t *testing.T) {
	out := os.Getenv("PZ_OUT")
	if out == "" {
		t.Skip("defina PZ_OUT para gerar o arquivo de exemplo")
	}
	media := gameMedia(t)
	body := loadFile(t, filepath.Join(media, "models_X/Skinned/MaleBody.x"))
	idle := loadFile(t, filepath.Join(media, "anims_X/Bob/Bob_Idle.x"))
	b := NewBuilder()
	b.AddSkeleton(body)
	tex, _ := os.ReadFile(filepath.Join(media, "textures/Body/MaleBody01.png"))
	if err := b.AddMesh(body, Material{Name: "pele", PNG: tex, Cutoff: 0.5}); err != nil {
		t.Fatal(err)
	}
	if err := b.AddAnimation(idle, "idle"); err != nil {
		t.Fatal(err)
	}
	glb, err := b.GLB()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, glb, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("gravado %s (%d KB)", out, len(glb)/1024)
}

// TestLocalWriteCharacters writes a few random characters when PZ_OUTDIR is set.
func TestLocalWriteCharacters(t *testing.T) {
	dir := os.Getenv("PZ_OUTDIR")
	if dir == "" {
		t.Skip("defina PZ_OUTDIR para gerar personagens de exemplo")
	}
	g, err := openGame(filepath.Dir(gameMedia(t)))
	if err != nil {
		t.Fatal(err)
	}
	c, err := g.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("roupas prontas: %d masc, %d fem; cabelos %d/%d; barbas %d", len(c.MaleOutfits), len(c.FemOutfits), len(c.MaleHair), len(c.FemHair), len(c.Beards))
	for _, seed := range []uint64{1, 2, 3, 4} {
		ch, err := c.NewCharacter(seed)
		if err != nil {
			t.Fatal(err)
		}
		glb, err := c.GLB(ch)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		t.Logf("seed %d: %s, feminino=%v, roupa=%s, pele=%d, cabelo=%s, barba=%s, peças3D=%d, pintadas=%d, %d KB",
			seed, ch.Name, ch.Female, ch.Outfit, ch.Skin, ch.Hair, ch.Beard, len(ch.models), len(ch.overlays), len(glb)/1024)
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("char%d.glb", seed)), glb, 0o644)
	}
}

func TestLocalProps(t *testing.T) {
	g, err := openGame(filepath.Dir(gameMedia(t)))
	if err != nil {
		t.Fatal(err)
	}
	for id := range Props {
		p, ok := g.Prop(id)
		if !ok {
			t.Errorf("%s: não encontrado", id)
			continue
		}
		t.Logf("%-10s escala=%.2f textura=%v", id, p.Scale, filepath.Base(p.Texture))
	}
}
