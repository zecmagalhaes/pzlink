package pzasset

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Props are the scenery objects of the lobby. Only these ids can be served,
// and only from the player's own game folder.
var Props = map[string]string{
	"barraca":  "WorldItems/Tent",
	"gerador":  "WorldItems/Generator",
	"caixote":  "WorldItems/Crate_Basic",
	"lampiao":  "WorldItems/Lamp",
	"fogueira": "WorldItems/CampfireMaterials",
	"lenha":    "WorldItems/Firewood_Bundle",
	"radio":    "WorldItems/Radio",
}

// propTexture covers props whose texture the game scripts do not name.
var propTexture = map[string]string{
	"caixote": "WorldItems/CrateBasic",
}

type PropInfo struct {
	Model   string  // file path of the .fbx
	Texture string  // file path of the .png ("" if unknown)
	Scale   float64 // scale from the game's model script (1 if unknown)
}

var (
	modelStartRe = regexp.MustCompile(`(?i)\bmodel\s+\w+\s*\{`)
	fieldRe      = regexp.MustCompile(`(?i)\b(mesh|texture|scale)\s*=\s*([^,\r\n}]+)`)
)

// modelBlocks returns the top-level text of every "model X { ... }" block,
// without its nested blocks (attachments).
func modelBlocks(src string) []string {
	var out []string
	for _, loc := range modelStartRe.FindAllStringIndex(src, -1) {
		depth := 1
		var b strings.Builder
		for i := loc[1]; i < len(src) && depth > 0; i++ {
			switch src[i] {
			case '{':
				depth++
			case '}':
				depth--
			default:
				if depth == 1 {
					b.WriteByte(src[i])
				}
			}
		}
		out = append(out, b.String())
	}
	return out
}

type modelDef struct {
	texture string
	scale   float64
}

// modelDefs reads "model X { mesh = ..., texture = ..., scale = ... }" blocks
// from the game scripts, keyed by lowercased mesh.
func (g *Game) modelDefs() map[string]modelDef {
	g.defsOnce.Do(func() {
		g.defs = map[string]modelDef{}
		filepath.WalkDir(filepath.Join(g.Root, "media", "scripts"), func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.EqualFold(filepath.Ext(p), ".txt") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for _, block := range modelBlocks(string(b)) {
				var mesh string
				d := modelDef{scale: 1}
				for _, f := range fieldRe.FindAllStringSubmatch(block, -1) {
					v := strings.TrimSpace(f[2])
					switch strings.ToLower(f[1]) {
					case "mesh":
						mesh = strings.ToLower(v)
					case "texture":
						d.texture = v
					case "scale":
						if s, err := strconv.ParseFloat(v, 64); err == nil && s > 0 {
							d.scale = s
						}
					}
				}
				if mesh != "" {
					if _, dup := g.defs[mesh]; !dup {
						g.defs[mesh] = d
					}
				}
			}
			return nil
		})
	})
	return g.defs
}

// Prop resolves a lobby prop id.
func (g *Game) Prop(id string) (PropInfo, bool) {
	mesh, ok := Props[id]
	if !ok {
		return PropInfo{}, false
	}
	model, ok := g.Path("media/models_X/" + mesh + ".fbx")
	if !ok {
		return PropInfo{}, false
	}
	info := PropInfo{Model: model, Scale: 1}
	d, ok := g.modelDefs()[strings.ToLower(mesh)]
	if ok {
		info.Scale = d.scale
		if t, ok := g.TexturePath(d.texture); ok {
			info.Texture = t
		}
	}
	if t, ok := propTexture[id]; ok && info.Texture == "" {
		if p, ok := g.TexturePath(t); ok {
			info.Texture = p
		}
	}
	if info.Texture == "" {
		if t, ok := g.TexturePath(mesh); ok {
			info.Texture = t
		}
	}
	return info, true
}
