package pzasset

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

// A tiny hand-written .X file (no game data): one bone, one skinned triangle
// referencing a top-level material, and a two-key rotation animation.
const tinyX = `xof 0303txt 0032
template Frame {
 <3d82ab46-62da-11cf-ab39-0020af71e433>
 [...]
}
Material mat_1 {
 1.0;1.0;1.0;1.0;;
 1.0;
 0.0;0.0;0.0;;
 0.0;0.0;0.0;;
 TextureFilename { "pele.png"; }
}
Frame Raiz {
 FrameTransformMatrix { 1,0,0,0, 0,1,0,0, 0,0,1,0, 0,0,0,1;; }
 Frame Osso {
  FrameTransformMatrix { 1,0,0,0, 0,1,0,0, 0,0,1,0, 0,0,2,1;; }
 }
 Mesh Tri {
  3;
  0.0;0.0;0.0;,
  1.0;0.0;0.0;,
  0.0;1.0;0.0;;
  1;
  3;0,1,2;;
  MeshNormals { 1; 0.0;0.0;1.0;; 1; 3;0,0,0;; }
  MeshTextureCoords { 3; 0.0;0.0;, 1.0;0.0;, 0.0;1.0;; }
  MeshMaterialList { 1; 1; 0;; { mat_1 } }
  SkinWeights {
   "Osso";
   3;
   0,1,2;
   1.0,1.0,1.0;
   1,0,0,0, 0,1,0,0, 0,0,1,0, 0,0,-2,1;;
  }
 }
}
AnimTicksPerSecond { 100; }
AnimationSet Gira {
 Animation {
  { Osso }
  AnimationKey { 0; 2; 0;4;1.0,0.0,0.0,0.0;;, 100;4;0.7071,0.0,0.7071,0.0;;; }
 }
}
`

func TestTinyX(t *testing.T) {
	s, err := LoadX(strings.NewReader(tinyX))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Meshes) != 1 || s.Meshes[0].Texture != "pele.png" || len(s.Meshes[0].Skins) != 1 {
		t.Fatalf("malha lida errado: %+v", s.Meshes)
	}
	if f := s.FindFrame("Osso"); f == nil || f.Matrix[14] != 2 {
		t.Fatal("hierarquia lida errado")
	}
	a := s.Animations
	if len(a) != 1 || a[0].TicksPerSecond != 100 || len(a[0].Tracks) != 1 || len(a[0].Tracks[0].Rot) != 2 {
		t.Fatalf("animação lida errado: %+v", a)
	}

	b := NewBuilder()
	b.AddSkeleton(s)
	if err := b.AddMesh(s, Material{Name: "m"}); err != nil {
		t.Fatal(err)
	}
	if err := b.AddAnimation(s, "gira"); err != nil {
		t.Fatal(err)
	}
	glb, err := b.GLB()
	if err != nil {
		t.Fatal(err)
	}
	if string(glb[:4]) != "glTF" || binary.LittleEndian.Uint32(glb[8:]) != uint32(len(glb)) {
		t.Fatal("cabeçalho GLB inválido")
	}
	n := binary.LittleEndian.Uint32(glb[12:])
	var doc struct {
		Nodes []struct {
			Name        string    `json:"name"`
			Translation []float64 `json:"translation"`
		} `json:"nodes"`
		Animations []struct {
			Channels []struct {
				Target struct{ Path string } `json:"target"`
			} `json:"channels"`
		} `json:"animations"`
		Skins []struct{ Joints []int } `json:"skins"`
	}
	if err := json.NewDecoder(bytes.NewReader(glb[20 : 20+n])).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	var osso []float64
	for _, nd := range doc.Nodes {
		if nd.Name == "Osso" {
			osso = nd.Translation
		}
	}
	// .X is left-handed: Z must be mirrored
	if len(osso) != 3 || osso[2] != -2 {
		t.Fatalf("Z não espelhado: %v", osso)
	}
	if len(doc.Skins) != 1 || len(doc.Animations) != 1 || doc.Animations[0].Channels[0].Target.Path != "rotation" {
		t.Fatalf("glTF incompleto: %+v", doc)
	}
}

func TestRejectsBadX(t *testing.T) {
	for _, src := range []string{"", "nao e x", "xof 0303bin 0032\n", "xof 0303txt 0032\nMesh m { 999999999; }", "xof 0303txt 0032\nFrame a {"} {
		if _, err := LoadX(strings.NewReader(src)); err == nil {
			t.Errorf("deveria recusar %q", src)
		}
	}
}
