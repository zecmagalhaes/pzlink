package pzasset

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// Game is a Project Zomboid installation on this PC. Files are looked up
// case-insensitively because the game's own references mix cases.
type Game struct {
	Root  string // folder that contains "media"
	once  sync.Once
	index map[string]string // lowercased path relative to Root, with "/" -> real path
	err   error

	defsOnce sync.Once
	defs     map[string]modelDef
}

const gameFolder = "ProjectZomboid"

// FindGame locates the game. PZ_HOME (the folder containing "media") wins;
// otherwise every Steam library on the machine is checked.
func FindGame() (*Game, error) {
	if h := os.Getenv("PZ_HOME"); h != "" {
		return openGame(h)
	}
	for _, lib := range steamLibraries() {
		dir := filepath.Join(lib, "steamapps", "common", gameFolder)
		if g, err := openGame(dir); err == nil {
			return g, nil
		}
	}
	return nil, errors.New("Project Zomboid não encontrado neste PC")
}

func openGame(dir string) (*Game, error) {
	if st, err := os.Stat(filepath.Join(dir, "media", "models_X")); err != nil || !st.IsDir() {
		return nil, errors.New("pasta do jogo inválida")
	}
	return &Game{Root: dir}, nil
}

var vdfPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)

// steamLibraries lists Steam library folders from the default locations and
// libraryfolders.vdf.
func steamLibraries() []string {
	var roots []string
	if runtime.GOOS == "windows" {
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
			if p := os.Getenv(env); p != "" {
				roots = append(roots, filepath.Join(p, "Steam"))
			}
		}
		roots = append(roots, `C:\Steam`, `D:\Steam`, `D:\SteamLibrary`, `E:\SteamLibrary`)
	} else {
		home, _ := os.UserHomeDir()
		roots = append(roots,
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".local", "share", "Steam"),
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
		)
	}
	seen := map[string]bool{}
	var libs []string
	add := func(p string) {
		p = filepath.Clean(p)
		if !seen[strings.ToLower(p)] {
			seen[strings.ToLower(p)] = true
			libs = append(libs, p)
		}
	}
	for _, r := range roots {
		add(r)
		f, err := os.Open(filepath.Join(r, "steamapps", "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if m := vdfPath.FindStringSubmatch(sc.Text()); m != nil {
				add(strings.ReplaceAll(m[1], `\\`, `\`))
			}
		}
		f.Close()
	}
	return libs
}

// indexed folders under media (everything the lobby needs, nothing else)
var indexDirs = []string{"models_X/Skinned", "models_X/Static/Clothes", "models_X/WorldItems", "textures", "clothing", "hairStyles", "anims_X/Bob"}

func (g *Game) buildIndex() {
	g.index = map[string]string{}
	for _, d := range indexDirs {
		base := filepath.Join(g.Root, "media", filepath.FromSlash(d))
		filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(g.Root, p)
			if err != nil {
				return nil
			}
			g.index[strings.ToLower(filepath.ToSlash(rel))] = p
			return nil
		})
	}
	g.index[strings.ToLower("media/fileGuidTable.xml")] = filepath.Join(g.Root, "media", "fileGuidTable.xml")
	if len(g.index) < 10 {
		g.err = errors.New("arquivos do jogo não encontrados")
	}
}

// Path resolves a game-relative path such as "media/textures/Body/MaleBody01.png"
// (any case, "\" or "/") to a real file. Only indexed folders are reachable.
func (g *Game) Path(rel string) (string, bool) {
	g.once.Do(g.buildIndex)
	key := strings.ToLower(strings.ReplaceAll(rel, `\`, "/"))
	key = strings.TrimPrefix(key, "./")
	p, ok := g.index[key]
	return p, ok
}

// ModelPath resolves a model reference from the clothing or hair files.
func (g *Game) ModelPath(ref string) (string, bool) {
	ref = strings.TrimSpace(strings.ReplaceAll(ref, `\`, "/"))
	if ref == "" || strings.EqualFold(ref, "null") {
		return "", false
	}
	if !strings.HasSuffix(strings.ToLower(ref), ".x") {
		ref += ".x"
	}
	if strings.HasPrefix(strings.ToLower(ref), "media/") {
		return g.Path(ref)
	}
	return g.Path("media/models_X/" + ref)
}

// TexturePath resolves a texture reference ("clothes\hat\baseballcapred").
func (g *Game) TexturePath(ref string) (string, bool) {
	ref = strings.TrimSpace(strings.ReplaceAll(ref, `\`, "/"))
	if ref == "" {
		return "", false
	}
	if !strings.HasSuffix(strings.ToLower(ref), ".png") {
		ref += ".png"
	}
	if strings.HasPrefix(strings.ToLower(ref), "media/") {
		return g.Path(ref)
	}
	return g.Path("media/textures/" + ref)
}

func (g *Game) loadX(path string) (*Scene, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadX(f)
}
