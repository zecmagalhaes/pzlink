// Package pzasset reads Project Zomboid assets from the player's own game
// installation and converts them for the lobby. No game asset is ever stored
// in this repository or in the PZLink binary.
package pzasset

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// xObject is one data object of a DirectX .X text file.
type xObject struct {
	Type     string
	Name     string
	Data     []xValue // numbers and strings, in order
	Children []*xObject
	Refs     []string // { Name } references (used by Animation)
}

type xValue struct {
	Num   float64
	Str   string
	IsStr bool
}

func (o *xObject) child(t string) *xObject {
	for _, c := range o.Children {
		if strings.EqualFold(c.Type, t) {
			return c
		}
	}
	return nil
}

func (o *xObject) childrenOf(t string) []*xObject {
	var out []*xObject
	for _, c := range o.Children {
		if strings.EqualFold(c.Type, t) {
			out = append(out, c)
		}
	}
	return out
}

// reader walks the flat data list of an object.
type reader struct {
	d   []xValue
	i   int
	err error
}

func (r *reader) num() float64 {
	if r.err != nil {
		return 0
	}
	if r.i >= len(r.d) || r.d[r.i].IsStr {
		r.err = fmt.Errorf("pzasset: dado numérico esperado na posição %d", r.i)
		return 0
	}
	v := r.d[r.i].Num
	r.i++
	return v
}

func (r *reader) int() int { return int(r.num()) }

func (r *reader) str() string {
	if r.err != nil {
		return ""
	}
	if r.i >= len(r.d) || !r.d[r.i].IsStr {
		r.err = fmt.Errorf("pzasset: texto esperado na posição %d", r.i)
		return ""
	}
	v := r.d[r.i].Str
	r.i++
	return v
}

// count reads an element count and rejects absurd values from damaged files.
func (r *reader) count(max int) int {
	n := r.int()
	if r.err == nil && (n < 0 || n > max) {
		r.err = fmt.Errorf("pzasset: contagem inválida %d", n)
		return 0
	}
	return n
}

// ---- tokenizer ----

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tNum
	tStr
	tLBrace
	tRBrace
)

type token struct {
	kind tokKind
	text string
	num  float64
}

type lexer struct {
	r    *bufio.Reader
	peek *token
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '-'
}

func (l *lexer) next() (token, error) {
	if l.peek != nil {
		t := *l.peek
		l.peek = nil
		return t, nil
	}
	for {
		c, err := l.r.ReadByte()
		if err == io.EOF {
			return token{kind: tEOF}, nil
		}
		if err != nil {
			return token{}, err
		}
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ';' || c == ',':
			continue
		case c == '/' || c == '#':
			if c == '/' {
				if n, _ := l.r.Peek(1); len(n) == 0 || n[0] != '/' {
					continue
				}
			}
			l.r.ReadString('\n')
			continue
		case c == '<': // template GUID
			l.r.ReadString('>')
			continue
		case c == '[': // template restrictions
			l.r.ReadString(']')
			continue
		case c == '{':
			return token{kind: tLBrace}, nil
		case c == '}':
			return token{kind: tRBrace}, nil
		case c == '"':
			s, err := l.r.ReadString('"')
			if err != nil {
				return token{}, errors.New("pzasset: texto sem fim")
			}
			return token{kind: tStr, text: s[:len(s)-1]}, nil
		case c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9'):
			var b strings.Builder
			b.WriteByte(c)
			for {
				n, err := l.r.Peek(1)
				if err != nil || len(n) == 0 {
					break
				}
				d := n[0]
				if (d >= '0' && d <= '9') || d == '.' || d == 'e' || d == 'E' || d == '-' || d == '+' {
					b.WriteByte(d)
					l.r.ReadByte()
					continue
				}
				break
			}
			f, err := strconv.ParseFloat(b.String(), 64)
			if err != nil {
				return token{}, fmt.Errorf("pzasset: número inválido %q", b.String())
			}
			return token{kind: tNum, num: f}, nil
		case isIdentStart(c):
			var b strings.Builder
			b.WriteByte(c)
			for {
				n, err := l.r.Peek(1)
				if err != nil || len(n) == 0 || !isIdentChar(n[0]) {
					break
				}
				b.WriteByte(n[0])
				l.r.ReadByte()
			}
			return token{kind: tIdent, text: b.String()}, nil
		default:
			continue
		}
	}
}

func (l *lexer) unread(t token) { l.peek = &t }

// parseX reads a text-format .X file into its top-level objects.
func parseX(r io.Reader) ([]*xObject, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	head, err := br.Peek(16)
	if err != nil || !strings.HasPrefix(string(head), "xof ") {
		return nil, errors.New("pzasset: não é um arquivo .X")
	}
	if !strings.Contains(string(head[8:12]), "txt") {
		return nil, errors.New("pzasset: só o formato .X de texto é suportado")
	}
	br.ReadString('\n')
	l := &lexer{r: br}
	var out []*xObject
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		if t.kind == tEOF {
			return out, nil
		}
		if t.kind != tIdent {
			return nil, errors.New("pzasset: objeto esperado")
		}
		if strings.EqualFold(t.text, "template") {
			if err := skipBlock(l); err != nil {
				return nil, err
			}
			continue
		}
		o, err := parseObject(l, t.text, 0)
		if err != nil {
			return nil, err
		}
		if o != nil && !strings.EqualFold(o.Type, "template") {
			out = append(out, o)
		}
	}
}

// skipBlock skips "Name { ... }" (a template declaration), including nesting.
func skipBlock(l *lexer) error {
	depth := 0
	for {
		t, err := l.next()
		if err != nil {
			return err
		}
		switch t.kind {
		case tEOF:
			return errors.New("pzasset: template sem fim")
		case tLBrace:
			depth++
		case tRBrace:
			depth--
			if depth == 0 {
				return nil
			}
		}
	}
}

const maxDepth = 64

func parseObject(l *lexer, typ string, depth int) (*xObject, error) {
	if depth > maxDepth {
		return nil, errors.New("pzasset: arquivo aninhado demais")
	}
	o := &xObject{Type: typ}
	t, err := l.next()
	if err != nil {
		return nil, err
	}
	if t.kind == tIdent {
		o.Name = t.text
		if t, err = l.next(); err != nil {
			return nil, err
		}
	}
	if t.kind != tLBrace {
		return nil, fmt.Errorf("pzasset: '{' esperado depois de %s", typ)
	}
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		switch t.kind {
		case tEOF:
			return nil, errors.New("pzasset: fim de arquivo inesperado")
		case tRBrace:
			return o, nil
		case tNum:
			o.Data = append(o.Data, xValue{Num: t.num})
		case tStr:
			o.Data = append(o.Data, xValue{Str: t.text, IsStr: true})
		case tLBrace: // reference: { Name }
			name, err := l.next()
			if err != nil {
				return nil, err
			}
			end, err := l.next()
			if err != nil {
				return nil, err
			}
			if name.kind != tIdent || end.kind != tRBrace {
				return nil, errors.New("pzasset: referência malformada")
			}
			o.Refs = append(o.Refs, name.text)
		case tIdent:
			c, err := parseObject(l, t.text, depth+1)
			if err != nil {
				return nil, err
			}
			o.Children = append(o.Children, c)
		}
	}
}
