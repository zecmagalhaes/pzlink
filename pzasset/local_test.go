package pzasset

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests read the game installed on the developer's PC and are skipped
// elsewhere (CI has no game files, and none may be committed).
func gameMedia(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("PZ_MEDIA")
	if dir == "" {
		dir = `C:\Program Files (x86)\Steam\steamapps\common\ProjectZomboid\media`
	}
	if _, err := os.Stat(filepath.Join(dir, "models_X")); err != nil {
		t.Skip("Project Zomboid não encontrado; defina PZ_MEDIA")
	}
	return dir
}

func loadFile(t *testing.T, path string) *Scene {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := LoadX(f)
	if err != nil {
		t.Fatalf("%s: %v", filepath.Base(path), err)
	}
	return s
}

func TestLocalParseAllCharacterFiles(t *testing.T) {
	media := gameMedia(t)
	var files []string
	for _, d := range []string{"models_X/Skinned/Clothes", "models_X/Skinned/Hair", "models_X/Skinned/Beards"} {
		m, _ := filepath.Glob(filepath.Join(media, d, "*.x"))
		files = append(files, m...)
	}
	for _, n := range []string{"MaleBody.x", "FemaleBody.x", "Male_Skeleton.x", "Female_Skeleton.x"} {
		files = append(files, filepath.Join(media, "models_X/Skinned", n))
	}
	anims, _ := filepath.Glob(filepath.Join(media, "anims_X/Bob/Bob_Idle*.x"))
	files = append(files, anims...)
	bad := 0
	for _, f := range files {
		fh, _ := os.Open(f)
		_, err := LoadX(fh)
		fh.Close()
		if err != nil {
			bad++
			if bad <= 10 {
				t.Errorf("%s: %v", filepath.Base(f), err)
			}
		}
	}
	t.Logf("%d arquivos lidos, %d com erro", len(files), bad)
}

func TestLocalBodyStructure(t *testing.T) {
	s := loadFile(t, filepath.Join(gameMedia(t), "models_X/Skinned/MaleBody.x"))
	if len(s.Meshes) != 1 {
		t.Fatalf("esperava 1 malha, veio %d", len(s.Meshes))
	}
	m := s.Meshes[0]
	t.Logf("malha %q: %d vértices, %d faces, %d normais, %d UVs, %d ossos com peso, textura %q, frame %v",
		m.Name, len(m.Positions), len(m.Faces), len(m.Normals), len(m.UVs), len(m.Skins), m.Texture, m.Frame != nil)
	// every vertex must be fully weighted
	sum := make([]float64, len(m.Positions))
	for _, sw := range m.Skins {
		if s.FindFrame(sw.Bone) == nil {
			t.Errorf("osso %q sem frame", sw.Bone)
		}
		for i, v := range sw.Indices {
			sum[v] += sw.Weights[i]
		}
	}
	for i, w := range sum {
		if math.Abs(w-1) > 0.02 {
			t.Fatalf("vértice %d com soma de pesos %.3f", i, w)
		}
	}
}

// quatToRowMatrix builds the 3x3 rotation for row vectors (v*M) from (w,x,y,z),
// optionally conjugated.
func quatToRowMatrix(q [4]float64, conj bool) [9]float64 {
	w, x, y, z := q[0], q[1], q[2], q[3]
	if conj {
		x, y, z = -x, -y, -z
	}
	// column-vector matrix R(q)
	c := [9]float64{
		1 - 2*(y*y+z*z), 2 * (x*y - w*z), 2 * (x*z + w*y),
		2 * (x*y + w*z), 1 - 2*(x*x+z*z), 2 * (y*z - w*x),
		2 * (x*z - w*y), 2 * (y*z + w*x), 1 - 2*(x*x+y*y),
	}
	// transpose → row-vector convention
	return [9]float64{c[0], c[3], c[6], c[1], c[4], c[7], c[2], c[5], c[8]}
}

// The rest pose in FrameTransformMatrix and the first rotation key of a static
// bone must agree; this tells which quaternion convention the exporter used.
func TestLocalQuaternionConvention(t *testing.T) {
	s := loadFile(t, filepath.Join(gameMedia(t), "anims_X/Bob/Bob_Idle.x"))
	if len(s.Animations) == 0 {
		t.Fatal("sem animação")
	}
	plain, conj, n := 0.0, 0.0, 0
	for _, tr := range s.Animations[0].Tracks {
		f := s.FindFrame(tr.Bone)
		if f == nil || len(tr.Rot) == 0 {
			continue
		}
		// upper-left 3x3 of the frame matrix, normalized per row (ignore scale)
		var fm [9]float64
		for r := 0; r < 3; r++ {
			l := math.Sqrt(f.Matrix[r*4]*f.Matrix[r*4] + f.Matrix[r*4+1]*f.Matrix[r*4+1] + f.Matrix[r*4+2]*f.Matrix[r*4+2])
			for c := 0; c < 3; c++ {
				fm[r*3+c] = f.Matrix[r*4+c] / l
			}
		}
		a, b := quatToRowMatrix(tr.Rot[0].Q, false), quatToRowMatrix(tr.Rot[0].Q, true)
		for i := range fm {
			plain += math.Abs(fm[i] - a[i])
			conj += math.Abs(fm[i] - b[i])
		}
		n++
	}
	t.Logf("%d ossos; erro médio sem conjugar=%.4f, conjugando=%.4f", n, plain/float64(n), conj/float64(n))
	if n == 0 {
		t.Fatal("nenhum osso comparado")
	}
}

func TestLocalTextureName(t *testing.T) {
	s := loadFile(t, filepath.Join(gameMedia(t), "models_X/Skinned/FemaleBody.x"))
	if !strings.HasSuffix(strings.ToLower(s.Meshes[0].Texture), ".png") {
		t.Fatalf("textura inesperada %q", s.Meshes[0].Texture)
	}
}
