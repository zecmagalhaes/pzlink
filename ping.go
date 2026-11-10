package main

import (
	"math"
	"sync"
)

const pingWindow = 60

type pingSample struct {
	ok      bool
	ms      float64
	direct  bool
	derp    string
	address string
}

// pingStats keeps a sliding window of ping results to one peer.
type pingStats struct {
	mu      sync.Mutex
	samples []pingSample
}

type pingSummary struct {
	Samples  int       `json:"samples"`
	LastMs   float64   `json:"lastMs"`
	AvgMs    float64   `json:"avgMs"`
	MinMs    float64   `json:"minMs"`
	MaxMs    float64   `json:"maxMs"`
	JitterMs float64   `json:"jitterMs"`
	LossPct  float64   `json:"lossPct"`
	Path     string    `json:"path"`
	Verdict  string    `json:"verdict"`
	Level    string    `json:"level"`
	History  []float64 `json:"history"`
}

func (p *pingStats) add(s pingSample) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.samples = append(p.samples, s)
	if len(p.samples) > pingWindow {
		p.samples = p.samples[len(p.samples)-pingWindow:]
	}
}

func (p *pingStats) summary() pingSummary {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out pingSummary
	out.Samples = len(p.samples)
	out.History = make([]float64, 0, len(p.samples))
	if out.Samples == 0 {
		out.Verdict, out.Level = "medindo...", "wait"
		return out
	}
	var sum, jitSum float64
	var ok, jitN int
	prev := -1.0
	out.MinMs = math.MaxFloat64
	for _, s := range p.samples {
		if !s.ok {
			out.History = append(out.History, -1)
			continue
		}
		out.History = append(out.History, s.ms)
		ok++
		sum += s.ms
		out.MinMs = math.Min(out.MinMs, s.ms)
		out.MaxMs = math.Max(out.MaxMs, s.ms)
		if prev >= 0 {
			jitSum += math.Abs(s.ms - prev)
			jitN++
		}
		prev = s.ms
	}
	out.LossPct = 100 * float64(out.Samples-ok) / float64(out.Samples)
	last := p.samples[len(p.samples)-1]
	for i := len(p.samples) - 1; i >= 0; i-- {
		if p.samples[i].ok {
			last = p.samples[i]
			break
		}
	}
	if ok == 0 {
		out.MinMs = 0
		out.Verdict, out.Level = "sem resposta", "bad"
		out.Path = "-"
		return out
	}
	out.LastMs = last.ms
	out.AvgMs = sum / float64(ok)
	if jitN > 0 {
		out.JitterMs = jitSum / float64(jitN)
	}
	if last.direct {
		out.Path = "direta"
	} else {
		out.Path = "via relay " + last.derp
	}

	switch {
	case out.Samples < 5:
		out.Verdict, out.Level = "medindo...", "wait"
	case out.AvgMs <= 80 && out.LossPct < 1 && out.JitterMs <= 20:
		out.Verdict, out.Level = "ótima para jogar", "good"
	case out.AvgMs <= 150 && out.LossPct < 3 && out.JitterMs <= 40:
		out.Verdict, out.Level = "jogável", "ok"
	default:
		out.Verdict, out.Level = "instável", "bad"
	}
	return out
}
