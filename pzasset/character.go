package pzasset

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
)

// Character is a randomly dressed survivor, reproducible from its seed.
type Character struct {
	Seed       uint64     `json:"seed"`
	Profession string     `json:"profession,omitempty"`
	Female     bool       `json:"female"`
	Name       string     `json:"name"`
	Outfit     string     `json:"outfit"`
	Skin       int        `json:"skin"`
	Hair       string     `json:"hair"`
	Beard      string     `json:"beard,omitempty"`
	HairColor  [3]float64 `json:"hairColor"`
	Items      []string   `json:"items,omitempty"` // clothing item names

	models   []piece
	overlays []overlay
	hair     *piece
	beard    *piece
}

type piece struct {
	model   string // file path of the .x
	texture string // file path of the .png
	tint    [3]float64
}

type overlay struct {
	texture string
	tint    [3]float64
}

var (
	firstMale   = []string{"André", "Bruno", "Carlos", "Diego", "Eduardo", "Felipe", "Gabriel", "Hugo", "Igor", "João", "Lucas", "Marcos", "Nicolas", "Otávio", "Pedro", "Rafael", "Samuel", "Tiago", "Vitor", "William", "Jack", "Frank", "Dale", "Earl", "Wayne"}
	firstFemale = []string{"Alice", "Beatriz", "Carla", "Daniela", "Elisa", "Fernanda", "Gabriela", "Helena", "Isabela", "Júlia", "Larissa", "Marina", "Natália", "Paula", "Renata", "Sofia", "Tatiana", "Vanessa", "Peggy", "Darlene", "Kate", "Linda"}
	lastNames   = []string{"Almeida", "Barbosa", "Costa", "Dias", "Ferreira", "Gomes", "Lima", "Martins", "Nogueira", "Oliveira", "Pereira", "Ramos", "Santos", "Teixeira", "Vieira", "Miller", "Baker", "Walker", "Turner", "Hayes", "Boone"}
	hairColors  = [][3]float64{
		{0.08, 0.06, 0.05}, {0.20, 0.13, 0.08}, {0.33, 0.21, 0.12}, {0.48, 0.32, 0.18},
		{0.70, 0.55, 0.30}, {0.85, 0.72, 0.45}, {0.55, 0.20, 0.08}, {0.62, 0.62, 0.60},
	}
	tints = [][3]float64{
		{0.85, 0.85, 0.85}, {0.25, 0.30, 0.45}, {0.45, 0.20, 0.18}, {0.30, 0.38, 0.25},
		{0.55, 0.48, 0.35}, {0.20, 0.20, 0.22}, {0.70, 0.62, 0.40}, {0.35, 0.45, 0.60},
	}
)

func pick[T any](r *rand.Rand, s []T) T { return s[r.IntN(len(s))] }

// Options steer the random survivor: Sex is "m", "f" or anything else for
// random; Profession picks outfits that fit it.
type Options struct {
	Sex        string
	Profession string
}

// NewCharacter dresses a random survivor. The same seed and options give the
// same result on every PC with the same game version.
func (c *Catalog) NewCharacter(seed uint64, opt Options) (*Character, error) {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	female := r.IntN(2) == 0
	switch opt.Sex {
	case "m":
		female = false
	case "f":
		female = true
	}
	ch := &Character{Seed: seed, Female: female, Skin: 1 + r.IntN(5), Profession: opt.Profession}
	g := c.game

	if ch.Female {
		ch.Name = pick(r, firstFemale) + " " + pick(r, lastNames)
	} else {
		ch.Name = pick(r, firstMale) + " " + pick(r, lastNames)
	}

	outfits := c.MaleOutfits
	if ch.Female {
		outfits = c.FemOutfits
	}
	if want := professionOutfits[opt.Profession]; len(want) > 0 {
		var fit []Outfit
		for _, o := range outfits {
			for _, w := range want {
				if strings.EqualFold(o.Name, w) {
					fit = append(fit, o)
				}
			}
		}
		if len(fit) > 0 {
			outfits = fit
		}
	}
	// addItem dresses one clothing item; it reports whether anything was added.
	addItem := func(guid string) bool {
		it, err := c.Item(guid)
		if err != nil || it.Static {
			return false // attached props (hats, glasses) come later
		}
		tint := [3]float64{1, 1, 1}
		if it.AllowTint {
			tint = pick(r, tints)
		}
		model := it.MaleModel
		if ch.Female {
			model = it.FemaleModel
		}
		if mp, ok := g.ModelPath(model); ok {
			choices := it.TextureChoices
			if len(choices) == 0 {
				choices = it.BaseTextures
			}
			if len(choices) == 0 {
				return false
			}
			tp, ok := g.TexturePath(pick(r, choices))
			if !ok {
				return false
			}
			ch.models = append(ch.models, piece{model: mp, texture: tp, tint: tint})
			ch.Items = append(ch.Items, c.ItemName(guid))
			return true
		}
		if len(it.BaseTextures) > 0 {
			if tp, ok := g.TexturePath(pick(r, it.BaseTextures)); ok {
				ch.overlays = append(ch.overlays, overlay{texture: tp, tint: tint})
				ch.Items = append(ch.Items, c.ItemName(guid))
				return true
			}
		}
		return false
	}
	for attempt := 0; attempt < 8 && len(ch.models)+len(ch.overlays) == 0; attempt++ {
		o := pick(r, outfits)
		ch.Outfit = o.Name
		ch.models, ch.overlays, ch.Items = nil, nil, nil
		for _, e := range o.Items {
			if e.Probability > 0 && r.Float64() > e.Probability {
				continue
			}
			guid := e.GUID
			if len(e.Alt) > 0 {
				guid = pick(r, append([]string{e.GUID}, e.Alt...))
			}
			addItem(guid)
		}
	}
	// Many outfits only sometimes include trousers or a shirt (the game fills
	// the gap with other rules). Nobody in the lobby goes out half naked.
	fill := func(pool []string, has func([]string) bool) {
		for tries := 0; tries < 6 && !has(ch.Items) && len(pool) > 0; tries++ {
			addItem(pick(r, pool))
		}
	}
	fill(c.bottoms, hasBottom)
	fill(c.tops, hasTop)

	ch.HairColor = pick(r, hairColors)
	styles := c.MaleHair
	if ch.Female {
		styles = c.FemHair
	}
	var withModel []HairStyle
	for _, h := range styles {
		// "Hat*" styles are flattened versions meant to sit under a hat
		if strings.TrimSpace(h.Model) != "" && !strings.HasPrefix(strings.ToLower(h.Name), "hat") {
			withModel = append(withModel, h)
		}
	}
	if len(withModel) > 0 {
		h := pick(r, withModel)
		ch.Hair = h.Name
		if mp, ok := g.ModelPath(h.Model); ok {
			if tp, ok := g.TexturePath(h.Texture); ok {
				ch.hair = &piece{model: mp, texture: tp, tint: ch.HairColor}
			}
		}
	}
	if !ch.Female && len(c.Beards) > 0 && r.Float64() < 0.4 {
		b := pick(r, c.Beards)
		if mp, ok := g.ModelPath(b.Model); ok {
			if tp, ok := g.TexturePath(b.Texture); ok {
				ch.Beard = b.Name
				ch.beard = &piece{model: mp, texture: tp, tint: ch.HairColor}
			}
		}
	}
	return ch, nil
}

// scene cache: parsed .x files are read-only and shared between characters.
var (
	sceneMu    sync.Mutex
	sceneCache = map[string]*Scene{}
)

func (g *Game) scene(path string) (*Scene, error) {
	sceneMu.Lock()
	defer sceneMu.Unlock()
	if s, ok := sceneCache[path]; ok {
		return s, nil
	}
	s, err := g.loadX(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(sceneCache) > 512 {
		sceneCache = map[string]*Scene{}
	}
	sceneCache[path] = s
	return s, nil
}

// GLB builds the character: body, clothing, hair and the idle animation.
func (c *Catalog) GLB(ch *Character) ([]byte, error) {
	g := c.game
	bodyName := "MaleBody"
	if ch.Female {
		bodyName = "FemaleBody"
	}
	bp, ok := g.Path("media/models_X/Skinned/" + bodyName + ".x")
	if !ok {
		return nil, errors.New("corpo não encontrado")
	}
	body, err := g.scene(bp)
	if err != nil {
		return nil, err
	}
	b := NewBuilder()
	b.AddSkeleton(body)

	skin, ok := g.TexturePath(fmt.Sprintf("Body/%s%02d", bodyName, ch.Skin))
	if !ok {
		return nil, errors.New("textura de pele não encontrada")
	}
	bodyPNG, err := composite(skin, ch.overlays)
	if err != nil {
		return nil, err
	}
	if err := b.AddMesh(body, Material{Name: "corpo", PNG: bodyPNG, Cutoff: 0.5}); err != nil {
		return nil, err
	}

	add := func(p *piece, name string) {
		s, err := g.scene(p.model)
		if err != nil {
			return // a broken piece must not break the whole character
		}
		tex, err := os.ReadFile(p.texture)
		if err != nil {
			return
		}
		b.AddMesh(s, Material{Name: name, PNG: tex, Cutoff: 0.5, TwoSide: true,
			Color: [4]float64{p.tint[0], p.tint[1], p.tint[2], 1}})
	}
	for i := range ch.models {
		add(&ch.models[i], "roupa")
	}
	if ch.hair != nil {
		add(ch.hair, "cabelo")
	}
	if ch.beard != nil {
		add(ch.beard, "barba")
	}

	if ip, ok := g.Path("media/anims_X/Bob/Bob_Idle.x"); ok {
		if idle, err := g.scene(ip); err == nil {
			b.AddAnimation(idle, "idle")
		}
	}
	return b.GLB()
}

// composite paints the clothing overlays (shirts, socks, shoes...) on the skin.
func composite(skinPath string, overlays []overlay) ([]byte, error) {
	base, err := decodePNG(skinPath)
	if err != nil {
		return nil, err
	}
	out := image.NewNRGBA(base.Bounds())
	draw.Draw(out, out.Bounds(), base, base.Bounds().Min, draw.Src)
	w, h := out.Bounds().Dx(), out.Bounds().Dy()
	for _, o := range overlays {
		img, err := decodePNG(o.texture)
		if err != nil {
			continue
		}
		ob := img.Bounds()
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				// nearest-neighbour scale when sizes differ
				sx := ob.Min.X + x*ob.Dx()/w
				sy := ob.Min.Y + y*ob.Dy()/h
				src := color.NRGBAModel.Convert(img.At(sx, sy)).(color.NRGBA)
				if src.A == 0 {
					continue
				}
				dst := out.NRGBAAt(x, y)
				a := float64(src.A) / 255
				mix := func(s uint8, t float64, d uint8) uint8 {
					v := float64(s)*t*a + float64(d)*(1-a)
					return uint8(min(255, v+0.5))
				}
				out.SetNRGBA(x, y, color.NRGBA{
					R: mix(src.R, o.tint[0], dst.R),
					G: mix(src.G, o.tint[1], dst.G),
					B: mix(src.B, o.tint[2], dst.B),
					A: uint8(min(255, float64(src.A)+float64(dst.A)*(1-a)+0.5)),
				})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

var (
	topWords    = []string{"tshirt", "shirt", "jacket", "sweater", "hoodie", "jumper", "coat", "dress", "vest", "tanktop", "boilersuit", "suit", "apron", "blouse", "poncho", "smock", "robe", "top", "gown", "overalls", "dungarees", "bodysuit"}
	bottomWords = []string{"trousers", "shorts", "skirt", "dress", "jeans", "pants", "boilersuit", "suit_", "overalls", "dungarees", "gown", "bodysuit", "leggings", "kilt"}
)

func itemsMatch(items, words []string) bool {
	for _, it := range items {
		l := strings.ToLower(it)
		for _, w := range words {
			if strings.Contains(l, w) {
				return true
			}
		}
	}
	return false
}

func hasTop(items []string) bool    { return itemsMatch(items, topWords) }
func hasBottom(items []string) bool { return itemsMatch(items, bottomWords) }
