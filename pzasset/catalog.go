package pzasset

import (
	"encoding/xml"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
)

// Catalog holds the game's clothing, outfit and hair definitions.
type Catalog struct {
	game        *Game
	guidPath    map[string]string // item GUID -> clothing item xml (game-relative)
	MaleOutfits []Outfit
	FemOutfits  []Outfit
	MaleHair    []HairStyle
	FemHair     []HairStyle
	Beards      []HairStyle

	tops, bottoms []string // everyday shirts and trousers, to complete outfits

	mu    sync.Mutex
	items map[string]*ClothingItem
}

type Outfit struct {
	Name  string        `xml:"m_Name"`
	Items []OutfitEntry `xml:"m_items"`
}

type OutfitEntry struct {
	Probability float64  `xml:"probability"`
	GUID        string   `xml:"itemGUID"`
	Alt         []string `xml:"subItems>itemGUID"`
}

type ClothingItem struct {
	MaleModel      string   `xml:"m_MaleModel"`
	FemaleModel    string   `xml:"m_FemaleModel"`
	Static         bool     `xml:"m_Static"`
	AllowTint      bool     `xml:"m_AllowRandomTint"`
	AttachBone     string   `xml:"m_AttachBone"`
	BaseTextures   []string `xml:"m_BaseTextures"`
	TextureChoices []string `xml:"textureChoices"`
}

type HairStyle struct {
	Name    string `xml:"name"`
	Model   string `xml:"model"`
	Texture string `xml:"texture"`
	Level   int    `xml:"level"`
}

func readXML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return xml.Unmarshal(b, v)
}

// LoadCatalog reads the definitions the lobby uses.
func (g *Game) LoadCatalog() (*Catalog, error) {
	c := &Catalog{game: g, guidPath: map[string]string{}, items: map[string]*ClothingItem{}}

	var guids struct {
		Files []struct {
			Path string `xml:"path"`
			GUID string `xml:"guid"`
		} `xml:"files"`
	}
	p, ok := g.Path("media/fileGuidTable.xml")
	if !ok {
		return nil, errors.New("fileGuidTable.xml não encontrado")
	}
	if err := readXML(p, &guids); err != nil {
		return nil, err
	}
	for _, f := range guids.Files {
		guid := strings.ToLower(f.GUID)
		c.guidPath[guid] = f.Path
		name := strings.ToLower(f.Path[strings.LastIndexAny(f.Path, "/\\")+1:])
		switch {
		case !strings.HasPrefix(f.Path, "media/clothing/clothingItems/"):
		case strings.HasPrefix(name, "trousers_") && !strings.ContainsAny(name, "0123456789") && !strings.Contains(name, "crafted") && !strings.Contains(name, "scrubs") &&
			!strings.Contains(name, "hazmat") && !strings.Contains(name, "prisoner") && !strings.Contains(name, "santa"):
			c.bottoms = append(c.bottoms, guid)
		case (strings.HasPrefix(name, "tshirt_") || strings.HasPrefix(name, "shirt_")) &&
			!strings.Contains(name, "profession") && !strings.Contains(name, "santa"):
			c.tops = append(c.tops, guid)
		}
	}
	sort.Strings(c.tops) // map order is random; the draw must be reproducible
	sort.Strings(c.bottoms)

	var outfits struct {
		Male   []Outfit `xml:"m_MaleOutfits"`
		Female []Outfit `xml:"m_FemaleOutfits"`
	}
	if p, ok = g.Path("media/clothing/clothing.xml"); !ok {
		return nil, errors.New("clothing.xml não encontrado")
	}
	if err := readXML(p, &outfits); err != nil {
		return nil, err
	}
	c.MaleOutfits, c.FemOutfits = playable(outfits.Male), playable(outfits.Female)

	var hair struct {
		Male   []HairStyle `xml:"male"`
		Female []HairStyle `xml:"female"`
	}
	if p, ok = g.Path("media/hairStyles/hairStyles.xml"); ok {
		readXML(p, &hair)
	}
	c.MaleHair, c.FemHair = hair.Male, hair.Female
	var beards struct {
		Styles []HairStyle `xml:"style"`
	}
	if p, ok = g.Path("media/hairStyles/beardStyles.xml"); ok {
		readXML(p, &beards)
	}
	c.Beards = beards.Styles
	if len(c.MaleOutfits) == 0 || len(c.FemOutfits) == 0 {
		return nil, errors.New("nenhuma roupa encontrada no jogo")
	}
	return c, nil
}

// playable drops test and costume-only outfits that look odd in a lobby.
func playable(in []Outfit) []Outfit {
	var out []Outfit
	for _, o := range in {
		n := strings.ToLower(o.Name)
		if strings.Contains(n, "test") || strings.HasPrefix(n, "armortest") || strings.HasPrefix(n, "1rj") ||
			strings.Contains(n, "naked") || strings.Contains(n, "hazmat") || strings.HasPrefix(n, "costume") ||
			strings.Contains(n, "bedroom") || strings.Contains(n, "bathrobe") || strings.Contains(n, "stripper") {
			continue
		}
		if len(o.Items) > 0 {
			out = append(out, o)
		}
	}
	return out
}

// Item returns the clothing item definition for a GUID.
func (c *Catalog) Item(guid string) (*ClothingItem, error) {
	guid = strings.ToLower(guid)
	c.mu.Lock()
	defer c.mu.Unlock()
	if it, ok := c.items[guid]; ok {
		return it, nil
	}
	rel, ok := c.guidPath[guid]
	if !ok {
		return nil, errors.New("peça desconhecida")
	}
	p, ok := c.game.Path(rel)
	if !ok {
		return nil, errors.New("arquivo da peça não encontrado")
	}
	it := &ClothingItem{}
	if err := readXML(p, it); err != nil {
		return nil, err
	}
	c.items[guid] = it
	return it, nil
}

// ItemName is the clothing item's file name without extension ("Tshirt_ArmyGreen").
func (c *Catalog) ItemName(guid string) string {
	rel := c.guidPath[strings.ToLower(guid)]
	rel = rel[strings.LastIndexAny(rel, "/\\")+1:]
	return strings.TrimSuffix(rel, ".xml")
}
