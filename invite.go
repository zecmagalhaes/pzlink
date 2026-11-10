package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

const invitePrefix = "PZL1-"

type invite struct {
	Key   string `json:"k"`
	Host  string `json:"h"`
	Node  string `json:"n,omitempty"` // node key of the host; pins its identity
	Ports []int  `json:"p"`
}

func encodeInvite(inv invite) string {
	b, _ := json.Marshal(inv)
	return invitePrefix + base64.RawURLEncoding.EncodeToString(b)
}

func decodeInvite(s string) (invite, error) {
	var inv invite
	s = strings.Join(strings.Fields(s), "")
	if !strings.HasPrefix(s, invitePrefix) {
		return inv, errors.New("código de convite inválido (deve começar com " + invitePrefix + ")")
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, invitePrefix))
	if err != nil {
		return inv, errors.New("código de convite corrompido; copie de novo")
	}
	if err := json.Unmarshal(b, &inv); err != nil || inv.Key == "" || inv.Host == "" || len(inv.Ports) == 0 {
		return inv, errors.New("código de convite incompleto; copie de novo")
	}
	if !strings.HasPrefix(inv.Key, "tskey-") || !strings.HasPrefix(inv.Host, hostPrefix) {
		return inv, errors.New("código de convite inválido")
	}
	if inv.Node != "" && !strings.HasPrefix(inv.Node, "nodekey:") {
		return inv, errors.New("código de convite inválido")
	}
	if len(inv.Ports) > 8 {
		return inv, errors.New("código de convite inválido (portas demais)")
	}
	for _, p := range inv.Ports {
		if p < 1024 || p > 65535 {
			return inv, errors.New("código de convite inválido (porta fora do permitido)")
		}
	}
	return inv, nil
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}
