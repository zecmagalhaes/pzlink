package pzasset

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"sort"
)

// Conversion from DirectX (.X) to glTF:
//   - .X matrices are row-major for row vectors; read as column-major they are
//     already the column-vector matrix glTF wants.
//   - .X is left-handed: mirror Z (S = diag(1,1,-1)) on positions, normals and
//     matrices (S·M·S), and reverse the triangle winding.
//   - .X animation quaternions (w,x,y,z) are the conjugate of the rotation
//     (verified against the rest pose in TestLocalQuaternionConvention);
//     after conjugating and mirroring Z the glTF quaternion is [x, y, -z, w].

func convMatrix(m [16]float64) [16]float64 {
	var o [16]float64
	for i, v := range m {
		c, r := i/4, i%4
		if (r == 2) != (c == 2) {
			v = -v
		}
		o[i] = v
	}
	return o
}

func convVec(v [3]float64) [3]float64 { return [3]float64{v[0], v[1], -v[2]} }

func convQuat(q [4]float64) [4]float64 { // in: w,x,y,z (.X) out: x,y,z,w (glTF)
	return [4]float64{q[1], q[2], -q[3], q[0]}
}

// ---- glTF JSON types (only what is needed) ----

type gNode struct {
	Name     string    `json:"name,omitempty"`
	Children []int     `json:"children,omitempty"`
	Matrix   []float64 `json:"matrix,omitempty"`
	Mesh     *int      `json:"mesh,omitempty"`
	Skin     *int      `json:"skin,omitempty"`
	Extras   any       `json:"extras,omitempty"`
}

type gPrim struct {
	Attributes map[string]int `json:"attributes"`
	Indices    int            `json:"indices"`
	Material   *int           `json:"material,omitempty"`
}

type gMesh struct {
	Name       string  `json:"name,omitempty"`
	Primitives []gPrim `json:"primitives"`
}

type gSkin struct {
	Joints              []int `json:"joints"`
	InverseBindMatrices int   `json:"inverseBindMatrices"`
	Skeleton            *int  `json:"skeleton,omitempty"`
}

type gAccessor struct {
	BufferView    int       `json:"bufferView"`
	ComponentType int       `json:"componentType"`
	Count         int       `json:"count"`
	Type          string    `json:"type"`
	Normalized    bool      `json:"normalized,omitempty"`
	Min           []float64 `json:"min,omitempty"`
	Max           []float64 `json:"max,omitempty"`
}

type gView struct {
	Buffer     int `json:"buffer"`
	ByteOffset int `json:"byteOffset"`
	ByteLength int `json:"byteLength"`
}

type gMaterial struct {
	Name   string   `json:"name,omitempty"`
	PBR    gPBR     `json:"pbrMetallicRoughness"`
	Alpha  string   `json:"alphaMode,omitempty"`
	Cutoff *float64 `json:"alphaCutoff,omitempty"`
	Double bool     `json:"doubleSided,omitempty"`
}

type gPBR struct {
	BaseColorFactor  []float64 `json:"baseColorFactor,omitempty"`
	BaseColorTexture *gTexRef  `json:"baseColorTexture,omitempty"`
	Metallic         float64   `json:"metallicFactor"`
	Roughness        float64   `json:"roughnessFactor"`
}

type gTexRef struct {
	Index int `json:"index"`
}

type gChannel struct {
	Sampler int `json:"sampler"`
	Target  struct {
		Node int    `json:"node"`
		Path string `json:"path"`
	} `json:"target"`
}

type gSampler struct {
	Input         int    `json:"input"`
	Output        int    `json:"output"`
	Interpolation string `json:"interpolation"`
}

type gAnimation struct {
	Name     string     `json:"name,omitempty"`
	Channels []gChannel `json:"channels"`
	Samplers []gSampler `json:"samplers"`
}

// Builder assembles one glTF binary (GLB).
type Builder struct {
	nodes     []gNode
	meshes    []gMesh
	skins     []gSkin
	accessors []gAccessor
	views     []gView
	materials []gMaterial
	textures  []map[string]int
	images    []map[string]any
	samplers  []map[string]int
	anims     []gAnimation
	scene     []int
	bin       bytes.Buffer
	byName    map[string]int         // frame name -> node index (the skeleton)
	parentOf  map[string]string      // frame name -> parent frame name
	bindOf    map[string][16]float64 // inverse bind matrices seen so far, by bone
}

func NewBuilder() *Builder {
	return &Builder{byName: map[string]int{}, parentOf: map[string]string{}, bindOf: map[string][16]float64{}}
}

func (b *Builder) view(data []byte) int {
	for b.bin.Len()%4 != 0 {
		b.bin.WriteByte(0)
	}
	b.views = append(b.views, gView{ByteOffset: b.bin.Len(), ByteLength: len(data)})
	b.bin.Write(data)
	return len(b.views) - 1
}

func (b *Builder) floats(vals []float32, typ string, count int, minmax bool) int {
	buf := make([]byte, 4*len(vals))
	for i, v := range vals {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	a := gAccessor{BufferView: b.view(buf), ComponentType: 5126, Count: count, Type: typ}
	if minmax && count > 0 {
		n := len(vals) / count
		a.Min, a.Max = make([]float64, n), make([]float64, n)
		for c := 0; c < n; c++ {
			a.Min[c], a.Max[c] = math.Inf(1), math.Inf(-1)
		}
		for i, v := range vals {
			c := i % n
			a.Min[c] = math.Min(a.Min[c], float64(v))
			a.Max[c] = math.Max(a.Max[c], float64(v))
		}
	}
	b.accessors = append(b.accessors, a)
	return len(b.accessors) - 1
}

func (b *Builder) uint16s(vals []uint16, typ string, count int) int {
	buf := make([]byte, 2*len(vals))
	for i, v := range vals {
		binary.LittleEndian.PutUint16(buf[i*2:], v)
	}
	b.accessors = append(b.accessors, gAccessor{BufferView: b.view(buf), ComponentType: 5123, Count: count, Type: typ})
	return len(b.accessors) - 1
}

func (b *Builder) uint32s(vals []uint32) int {
	buf := make([]byte, 4*len(vals))
	for i, v := range vals {
		binary.LittleEndian.PutUint32(buf[i*4:], v)
	}
	b.accessors = append(b.accessors, gAccessor{BufferView: b.view(buf), ComponentType: 5125, Count: len(vals), Type: "SCALAR"})
	return len(b.accessors) - 1
}

// AddSkeleton adds the frame hierarchy of s as nodes. Call once, with the
// scene that defines the character's skeleton.
func (b *Builder) AddSkeleton(s *Scene) {
	var add func(f *Frame) int
	add = func(f *Frame) int {
		m := convMatrix(f.Matrix)
		b.nodes = append(b.nodes, gNode{Name: f.Name, Matrix: m[:]})
		idx := len(b.nodes) - 1
		b.byName[f.Name] = idx
		if f.Parent != nil {
			b.parentOf[f.Name] = f.Parent.Name
		}
		for _, c := range f.Children {
			ci := add(c)
			b.nodes[idx].Children = append(b.nodes[idx].Children, ci)
		}
		return idx
	}
	for _, f := range s.Root {
		b.scene = append(b.scene, add(f))
	}
	// glTF animation targets need TRS, not a matrix; decompose.
	for i := range b.nodes {
		b.nodes[i].Matrix = nil
		b.nodes[i].Extras = nil
	}
	b.setTRS(s)
}

// node TRS kept separately so the JSON can carry translation/rotation/scale.
type trs struct {
	T [3]float64
	R [4]float64
	S [3]float64
}

func (b *Builder) setTRS(s *Scene) {
	var walk func(f *Frame)
	walk = func(f *Frame) {
		t := decompose(convMatrix(f.Matrix))
		b.nodes[b.byName[f.Name]].Extras = t
		for _, c := range f.Children {
			walk(c)
		}
	}
	for _, f := range s.Root {
		walk(f)
	}
}

// decompose splits a column-major affine matrix into TRS.
func decompose(m [16]float64) trs {
	var o trs
	o.T = [3]float64{m[12], m[13], m[14]}
	col := func(c int) [3]float64 { return [3]float64{m[c*4], m[c*4+1], m[c*4+2]} }
	l := func(v [3]float64) float64 { return math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2]) }
	c0, c1, c2 := col(0), col(1), col(2)
	sx, sy, sz := l(c0), l(c1), l(c2)
	// a negative determinant means a mirror; put it on X
	det := c0[0]*(c1[1]*c2[2]-c1[2]*c2[1]) - c1[0]*(c0[1]*c2[2]-c0[2]*c2[1]) + c2[0]*(c0[1]*c1[2]-c0[2]*c1[1])
	if det < 0 {
		sx = -sx
	}
	o.S = [3]float64{sx, sy, sz}
	r := [9]float64{}
	for i := 0; i < 3; i++ {
		r[i*3+0] = c0[i] / sx
		r[i*3+1] = c1[i] / sy
		r[i*3+2] = c2[i] / sz
	}
	o.R = matToQuat(r)
	return o
}

// matToQuat converts a row-major 3x3 rotation (column-vector convention) to x,y,z,w.
func matToQuat(m [9]float64) [4]float64 {
	tr := m[0] + m[4] + m[8]
	var x, y, z, w float64
	switch {
	case tr > 0:
		s := math.Sqrt(tr+1) * 2
		w, x, y, z = 0.25*s, (m[7]-m[5])/s, (m[2]-m[6])/s, (m[3]-m[1])/s
	case m[0] > m[4] && m[0] > m[8]:
		s := math.Sqrt(1+m[0]-m[4]-m[8]) * 2
		w, x, y, z = (m[7]-m[5])/s, 0.25*s, (m[1]+m[3])/s, (m[2]+m[6])/s
	case m[4] > m[8]:
		s := math.Sqrt(1+m[4]-m[0]-m[8]) * 2
		w, x, y, z = (m[2]-m[6])/s, (m[1]+m[3])/s, 0.25*s, (m[5]+m[7])/s
	default:
		s := math.Sqrt(1+m[8]-m[0]-m[4]) * 2
		w, x, y, z = (m[3]-m[1])/s, (m[2]+m[6])/s, (m[5]+m[7])/s, 0.25*s
	}
	n := math.Sqrt(x*x + y*y + z*z + w*w)
	return [4]float64{x / n, y / n, z / n, w / n}
}

// Material options for AddMesh.
type Material struct {
	Name    string
	PNG     []byte     // embedded base color texture (optional)
	Color   [4]float64 // multiplied with the texture; zero means white
	Blend   bool       // alpha blending (hair edges)
	Cutoff  float64    // alpha cutout when > 0
	TwoSide bool
}

func (b *Builder) addMaterial(m Material) int {
	gm := gMaterial{Name: m.Name, PBR: gPBR{Metallic: 0, Roughness: 0.9}, Double: m.TwoSide}
	if m.Color != ([4]float64{}) {
		gm.PBR.BaseColorFactor = m.Color[:]
	}
	if len(m.PNG) > 0 {
		v := b.view(m.PNG)
		b.images = append(b.images, map[string]any{"bufferView": v, "mimeType": "image/png"})
		if len(b.samplers) == 0 {
			b.samplers = append(b.samplers, map[string]int{"magFilter": 9729, "minFilter": 9987})
		}
		b.textures = append(b.textures, map[string]int{"source": len(b.images) - 1, "sampler": 0})
		gm.PBR.BaseColorTexture = &gTexRef{Index: len(b.textures) - 1}
	}
	switch {
	case m.Blend:
		gm.Alpha = "BLEND"
	case m.Cutoff > 0:
		c := m.Cutoff
		gm.Alpha, gm.Cutoff = "MASK", &c
	}
	b.materials = append(b.materials, gm)
	return len(b.materials) - 1
}

// AddMesh adds every mesh of s, skinned to the skeleton added with
// AddSkeleton (bones are matched by name).
func (b *Builder) AddMesh(s *Scene, mat Material) error {
	if len(b.byName) == 0 {
		return errors.New("pzasset: esqueleto não adicionado")
	}
	matIdx := b.addMaterial(mat)
	for _, m := range s.Meshes {
		if err := b.addOneMesh(m, matIdx); err != nil {
			return err
		}
	}
	return nil
}

func (b *Builder) addOneMesh(m *Mesh, matIdx int) error {
	type infl struct {
		joint int
		w     float64
	}
	// joints of this mesh, in skin order
	var joints []int
	var ibm []float32
	jointOf := map[string]int{}
	vinf := make([][]infl, len(m.Positions))
	offsetOf := map[string][16]float64{}
	for _, sw := range m.Skins {
		offsetOf[sw.Bone] = sw.Offset
		if _, ok := b.bindOf[sw.Bone]; !ok {
			b.bindOf[sw.Bone] = sw.Offset
		}
	}
	for _, sw := range m.Skins {
		bone := sw.Bone
		node, ok := b.byName[bone]
		if !ok {
			continue // bone missing from the base skeleton: ignore its weights
		}
		off, ok := offsetOf[bone]
		if !ok {
			if off, ok = b.bindOf[bone]; !ok {
				off = sw.Offset
			}
		}
		j, seen := jointOf[bone]
		if !seen {
			j = len(joints)
			jointOf[bone] = j
			joints = append(joints, node)
			om := convMatrix(off)
			for _, v := range om {
				ibm = append(ibm, float32(v))
			}
		}
		for i, vi := range sw.Indices {
			if sw.Weights[i] > 0 {
				vinf[vi] = append(vinf[vi], infl{j, sw.Weights[i]})
			}
		}
	}
	if len(joints) == 0 {
		return errors.New("pzasset: malha sem pesos de esqueleto")
	}
	if len(joints) > 65535 {
		return errors.New("pzasset: ossos demais")
	}

	// unify (position, normal) pairs into glTF vertices
	type key struct{ p, n int }
	index := map[key]uint32{}
	var pos, nor, uv, wts []float32
	var jts []uint16
	var idx []uint32
	vert := func(p, n int) uint32 {
		k := key{p, n}
		if v, ok := index[k]; ok {
			return v
		}
		v := uint32(len(pos) / 3)
		index[k] = v
		cp := convVec(m.Positions[p])
		pos = append(pos, float32(cp[0]), float32(cp[1]), float32(cp[2]))
		if n >= 0 {
			cn := convVec(m.Normals[n])
			nor = append(nor, float32(cn[0]), float32(cn[1]), float32(cn[2]))
		}
		if len(m.UVs) > 0 {
			uv = append(uv, float32(m.UVs[p][0]), float32(m.UVs[p][1]))
		}
		in := vinf[p]
		sort.Slice(in, func(i, j int) bool { return in[i].w > in[j].w })
		if len(in) > 4 {
			in = in[:4]
		}
		var sum float64
		for _, x := range in {
			sum += x.w
		}
		for i := 0; i < 4; i++ {
			if i < len(in) && sum > 0 {
				jts = append(jts, uint16(in[i].joint))
				wts = append(wts, float32(in[i].w/sum))
			} else if i == 0 {
				jts = append(jts, 0)
				wts = append(wts, 1)
			} else {
				jts = append(jts, 0)
				wts = append(wts, 0)
			}
		}
		return v
	}
	for fi, f := range m.Faces {
		var nf []int
		if len(m.NormFaces) > 0 {
			nf = m.NormFaces[fi]
		}
		ni := func(k int) int {
			if nf == nil || k >= len(nf) {
				return -1
			}
			return nf[k]
		}
		for k := 1; k+1 < len(f); k++ { // fan, winding reversed for the Z mirror
			a, c, d := vert(f[0], ni(0)), vert(f[k], ni(k)), vert(f[k+1], ni(k+1))
			idx = append(idx, a, d, c)
		}
	}
	if len(m.Normals) > 0 && len(nor) != len(pos) {
		return errors.New("pzasset: normais incompletas")
	}

	attrs := map[string]int{"POSITION": b.floats(pos, "VEC3", len(pos)/3, true)}
	if len(nor) > 0 {
		attrs["NORMAL"] = b.floats(nor, "VEC3", len(nor)/3, false)
	}
	if len(uv) > 0 {
		attrs["TEXCOORD_0"] = b.floats(uv, "VEC2", len(uv)/2, false)
	}
	attrs["JOINTS_0"] = b.uint16s(jts, "VEC4", len(jts)/4)
	attrs["WEIGHTS_0"] = b.floats(wts, "VEC4", len(wts)/4, false)
	mi := matIdx
	b.meshes = append(b.meshes, gMesh{Name: m.Name, Primitives: []gPrim{{Attributes: attrs, Indices: b.uint32s(idx), Material: &mi}}})
	meshIdx := len(b.meshes) - 1

	b.skins = append(b.skins, gSkin{Joints: joints, InverseBindMatrices: b.floats(ibm, "MAT4", len(joints), false)})
	skinIdx := len(b.skins) - 1
	b.nodes = append(b.nodes, gNode{Name: m.Name, Mesh: &meshIdx, Skin: &skinIdx})
	b.scene = append(b.scene, len(b.nodes)-1)
	return nil
}

// AddAnimation adds the first animation set of s, targeting skeleton bones by name.
func (b *Builder) AddAnimation(s *Scene, name string) error {
	if len(s.Animations) == 0 {
		return errors.New("pzasset: arquivo sem animação")
	}
	a := s.Animations[0]
	tps := a.TicksPerSecond
	if tps <= 0 {
		tps = 4800
	}
	ga := gAnimation{Name: name}
	channel := func(node int, path string, times []float32, vals []float32, typ string) {
		in := b.floats(times, "SCALAR", len(times), true)
		n := 3
		if typ == "VEC4" {
			n = 4
		}
		out := b.floats(vals, typ, len(vals)/n, false)
		ga.Samplers = append(ga.Samplers, gSampler{Input: in, Output: out, Interpolation: "LINEAR"})
		var ch gChannel
		ch.Sampler = len(ga.Samplers) - 1
		ch.Target.Node, ch.Target.Path = node, path
		ga.Channels = append(ga.Channels, ch)
	}
	for _, tr := range a.Tracks {
		node, ok := b.byName[tr.Bone]
		if !ok {
			continue
		}
		if len(tr.Rot) > 0 {
			var ts, vs []float32
			var prev [4]float64
			for i, k := range tr.Rot {
				q := convQuat(k.Q)
				if i > 0 && q[0]*prev[0]+q[1]*prev[1]+q[2]*prev[2]+q[3]*prev[3] < 0 {
					q = [4]float64{-q[0], -q[1], -q[2], -q[3]} // keep the shortest path
				}
				prev = q
				ts = append(ts, float32(k.T/tps))
				vs = append(vs, float32(q[0]), float32(q[1]), float32(q[2]), float32(q[3]))
			}
			channel(node, "rotation", ts, vs, "VEC4")
		}
		if len(tr.Pos) > 0 {
			var ts, vs []float32
			for _, k := range tr.Pos {
				v := convVec(k.V)
				ts = append(ts, float32(k.T/tps))
				vs = append(vs, float32(v[0]), float32(v[1]), float32(v[2]))
			}
			channel(node, "translation", ts, vs, "VEC3")
		}
		if len(tr.Scale) > 0 {
			var ts, vs []float32
			for _, k := range tr.Scale {
				ts = append(ts, float32(k.T/tps))
				vs = append(vs, float32(k.V[0]), float32(k.V[1]), float32(k.V[2]))
			}
			channel(node, "scale", ts, vs, "VEC3")
		}
	}
	if len(ga.Channels) == 0 {
		return errors.New("pzasset: animação não bate com o esqueleto")
	}
	b.anims = append(b.anims, ga)
	return nil
}

// GLB returns the binary glTF.
func (b *Builder) GLB() ([]byte, error) {
	type jsonNode struct {
		Name        string    `json:"name,omitempty"`
		Children    []int     `json:"children,omitempty"`
		Translation []float64 `json:"translation,omitempty"`
		Rotation    []float64 `json:"rotation,omitempty"`
		Scale       []float64 `json:"scale,omitempty"`
		Mesh        *int      `json:"mesh,omitempty"`
		Skin        *int      `json:"skin,omitempty"`
	}
	nodes := make([]jsonNode, len(b.nodes))
	for i, n := range b.nodes {
		jn := jsonNode{Name: n.Name, Children: n.Children, Mesh: n.Mesh, Skin: n.Skin}
		if t, ok := n.Extras.(trs); ok {
			jn.Translation, jn.Rotation, jn.Scale = t.T[:], t.R[:], t.S[:]
		}
		nodes[i] = jn
	}
	for i := range b.views {
		b.views[i].Buffer = 0
	}
	for b.bin.Len()%4 != 0 {
		b.bin.WriteByte(0)
	}
	doc := map[string]any{
		"asset":       map[string]string{"version": "2.0", "generator": "PZLink pzasset"},
		"scene":       0,
		"scenes":      []map[string]any{{"nodes": b.scene}},
		"nodes":       nodes,
		"buffers":     []map[string]int{{"byteLength": b.bin.Len()}},
		"bufferViews": b.views,
		"accessors":   b.accessors,
	}
	if len(b.meshes) > 0 {
		doc["meshes"] = b.meshes
		doc["skins"] = b.skins
		doc["materials"] = b.materials
	}
	if len(b.images) > 0 {
		doc["images"], doc["textures"], doc["samplers"] = b.images, b.textures, b.samplers
	}
	if len(b.anims) > 0 {
		doc["animations"] = b.anims
	}
	js, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	for len(js)%4 != 0 {
		js = append(js, ' ')
	}
	var out bytes.Buffer
	total := 12 + 8 + len(js) + 8 + b.bin.Len()
	binary.Write(&out, binary.LittleEndian, []uint32{0x46546C67, 2, uint32(total)})
	binary.Write(&out, binary.LittleEndian, []uint32{uint32(len(js)), 0x4E4F534A})
	out.Write(js)
	binary.Write(&out, binary.LittleEndian, []uint32{uint32(b.bin.Len()), 0x004E4942})
	out.Write(b.bin.Bytes())
	return out.Bytes(), nil
}
