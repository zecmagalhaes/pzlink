package pzasset

import (
	"fmt"
	"io"
	"strings"
)

// Scene is the content of one .X file, still in DirectX conventions
// (left-handed, row-vector matrices). The glTF writer converts it.
type Scene struct {
	mats       map[string]string // top-level materials: name -> texture
	Root       []*Frame
	Meshes     []*Mesh
	Animations []*Animation
}

type Frame struct {
	Name     string
	Matrix   [16]float64 // FrameTransformMatrix as stored (row-major, row vectors)
	Children []*Frame
	Parent   *Frame
}

type Mesh struct {
	Name      string
	Frame     *Frame // frame that owns the mesh (nil at top level)
	Positions [][3]float64
	Faces     [][]int
	Normals   [][3]float64
	NormFaces [][]int
	UVs       [][2]float64
	Texture   string
	Skins     []SkinWeights
}

type SkinWeights struct {
	Bone    string
	Indices []int
	Weights []float64
	Offset  [16]float64 // inverse bind matrix
}

type Animation struct {
	Name           string
	TicksPerSecond float64
	Tracks         []*Track
}

// Track holds the keys of one bone. Times are in ticks.
type Track struct {
	Bone  string
	Rot   []KeyQuat // .X order: w, x, y, z
	Scale []KeyVec
	Pos   []KeyVec
}

type KeyQuat struct {
	T float64
	Q [4]float64
}

type KeyVec struct {
	T float64
	V [3]float64
}

const (
	maxVerts = 1 << 20
	maxKeys  = 1 << 18
)

// LoadX parses a text .X file.
func LoadX(r io.Reader) (*Scene, error) {
	objs, err := parseX(r)
	if err != nil {
		return nil, err
	}
	s := &Scene{mats: map[string]string{}}
	for _, o := range objs {
		if strings.EqualFold(o.Type, "Material") && o.Name != "" {
			s.mats[o.Name] = materialTexture(o)
		}
	}
	ticks := 4800.0
	for _, o := range objs {
		switch strings.ToLower(o.Type) {
		case "frame":
			f, err := s.readFrame(o, nil)
			if err != nil {
				return nil, err
			}
			s.Root = append(s.Root, f)
		case "mesh":
			m, err := s.readMesh(o)
			if err != nil {
				return nil, err
			}
			s.Meshes = append(s.Meshes, m)
		case "animticksperseond", "animtickspersecond":
			if len(o.Data) > 0 && o.Data[0].Num > 0 {
				ticks = o.Data[0].Num
			}
		case "animationset":
			a, err := readAnimSet(o)
			if err != nil {
				return nil, err
			}
			s.Animations = append(s.Animations, a)
		}
	}
	for _, a := range s.Animations {
		a.TicksPerSecond = ticks
	}
	return s, nil
}

func readMatrix(o *xObject) ([16]float64, error) {
	var m [16]float64
	r := &reader{d: o.Data}
	for i := range m {
		m[i] = r.num()
	}
	return m, r.err
}

func (s *Scene) readFrame(o *xObject, parent *Frame) (*Frame, error) {
	f := &Frame{Name: o.Name, Parent: parent}
	f.Matrix = identity()
	for _, c := range o.Children {
		switch strings.ToLower(c.Type) {
		case "frametransformmatrix":
			m, err := readMatrix(c)
			if err != nil {
				return nil, err
			}
			f.Matrix = m
		case "frame":
			cf, err := s.readFrame(c, f)
			if err != nil {
				return nil, err
			}
			f.Children = append(f.Children, cf)
		case "mesh":
			m, err := s.readMesh(c)
			if err != nil {
				return nil, err
			}
			m.Frame = f
			s.Meshes = append(s.Meshes, m)
		}
	}
	return f, nil
}

func identity() [16]float64 {
	return [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

func readFaces(r *reader, nVerts int) [][]int {
	n := r.count(maxVerts * 2)
	faces := make([][]int, 0, n)
	for i := 0; i < n && r.err == nil; i++ {
		k := r.count(64)
		f := make([]int, k)
		for j := range f {
			f[j] = r.int()
			if r.err == nil && (f[j] < 0 || f[j] >= nVerts) {
				r.err = fmt.Errorf("pzasset: índice de vértice fora do limite")
			}
		}
		faces = append(faces, f)
	}
	return faces
}

func (s *Scene) readMesh(o *xObject) (*Mesh, error) {
	m := &Mesh{Name: o.Name}
	r := &reader{d: o.Data}
	nv := r.count(maxVerts)
	m.Positions = make([][3]float64, nv)
	for i := range m.Positions {
		m.Positions[i] = [3]float64{r.num(), r.num(), r.num()}
	}
	m.Faces = readFaces(r, nv)
	if r.err != nil {
		return nil, r.err
	}
	for _, c := range o.Children {
		cr := &reader{d: c.Data}
		switch strings.ToLower(c.Type) {
		case "meshnormals":
			nn := cr.count(maxVerts)
			m.Normals = make([][3]float64, nn)
			for i := range m.Normals {
				m.Normals[i] = [3]float64{cr.num(), cr.num(), cr.num()}
			}
			m.NormFaces = readFaces(cr, nn)
		case "meshtexturecoords":
			nt := cr.count(maxVerts)
			m.UVs = make([][2]float64, nt)
			for i := range m.UVs {
				m.UVs[i] = [2]float64{cr.num(), cr.num()}
			}
		case "meshmateriallist":
			for _, mat := range c.childrenOf("Material") {
				if tex := materialTexture(mat); tex != "" {
					m.Texture = tex
				}
			}
			for _, ref := range c.Refs {
				if tex := s.mats[ref]; tex != "" && m.Texture == "" {
					m.Texture = tex
				}
			}
		case "skinweights":
			var sw SkinWeights
			sw.Bone = cr.str()
			n := cr.count(maxVerts)
			sw.Indices = make([]int, n)
			for i := range sw.Indices {
				sw.Indices[i] = cr.int()
				if cr.err == nil && (sw.Indices[i] < 0 || sw.Indices[i] >= nv) {
					cr.err = fmt.Errorf("pzasset: peso aponta para vértice inexistente")
				}
			}
			sw.Weights = make([]float64, n)
			for i := range sw.Weights {
				sw.Weights[i] = cr.num()
			}
			for i := range sw.Offset {
				sw.Offset[i] = cr.num()
			}
			m.Skins = append(m.Skins, sw)
		}
		if cr.err != nil {
			return nil, fmt.Errorf("%s: %w", c.Type, cr.err)
		}
	}
	if len(m.Normals) > 0 && len(m.NormFaces) != len(m.Faces) {
		return nil, fmt.Errorf("pzasset: normais não batem com as faces")
	}
	if len(m.UVs) > 0 && len(m.UVs) != nv {
		return nil, fmt.Errorf("pzasset: coordenadas de textura não batem com os vértices")
	}
	return m, nil
}

func readAnimSet(o *xObject) (*Animation, error) {
	a := &Animation{Name: o.Name}
	for _, an := range o.childrenOf("Animation") {
		if len(an.Refs) == 0 {
			continue
		}
		tr := &Track{Bone: an.Refs[0]}
		for _, k := range an.childrenOf("AnimationKey") {
			r := &reader{d: k.Data}
			kind := r.int()
			n := r.count(maxKeys)
			for i := 0; i < n && r.err == nil; i++ {
				t := r.num()
				nvals := r.count(16)
				vals := make([]float64, nvals)
				for j := range vals {
					vals[j] = r.num()
				}
				switch {
				case kind == 0 && nvals == 4:
					tr.Rot = append(tr.Rot, KeyQuat{t, [4]float64{vals[0], vals[1], vals[2], vals[3]}})
				case kind == 1 && nvals == 3:
					tr.Scale = append(tr.Scale, KeyVec{t, [3]float64{vals[0], vals[1], vals[2]}})
				case kind == 2 && nvals == 3:
					tr.Pos = append(tr.Pos, KeyVec{t, [3]float64{vals[0], vals[1], vals[2]}})
				case kind == 4 && nvals == 16:
					return nil, fmt.Errorf("pzasset: animação por matriz não suportada (%s)", tr.Bone)
				}
			}
			if r.err != nil {
				return nil, fmt.Errorf("animação %s/%s: %w", a.Name, tr.Bone, r.err)
			}
		}
		a.Tracks = append(a.Tracks, tr)
	}
	return a, nil
}

// FindFrame returns the frame with the given name.
func (s *Scene) FindFrame(name string) *Frame {
	var walk func([]*Frame) *Frame
	walk = func(fs []*Frame) *Frame {
		for _, f := range fs {
			if f.Name == name {
				return f
			}
			if r := walk(f.Children); r != nil {
				return r
			}
		}
		return nil
	}
	return walk(s.Root)
}

func materialTexture(mat *xObject) string {
	if tf := mat.child("TextureFilename"); tf != nil && len(tf.Data) > 0 && tf.Data[0].IsStr {
		return tf.Data[0].Str
	}
	return ""
}
