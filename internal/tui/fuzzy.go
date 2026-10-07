package tui

import (
	"sort"
	"strings"
)

// fuzzyItems keeps items whose label contains every query rune, in order.
// Consecutive matches and matches at the start of a word rank higher.
func fuzzyItems(items []pickerItem, query string) []pickerItem {
	type hit struct {
		item  pickerItem
		score int
		index int
	}
	q := []rune(strings.ToLower(query))
	var hits []hit
	for i, it := range items {
		score, ok := fuzzyScore(q, it.label)
		if !ok {
			score, ok = fuzzyScore(q, it.id)
		}
		if !ok {
			continue
		}
		hits = append(hits, hit{item: it, score: score, index: i})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].index < hits[j].index
	})
	out := make([]pickerItem, len(hits))
	for i, h := range hits {
		out[i] = h.item
	}
	return out
}

func fuzzyScore(query []rune, target string) (int, bool) {
	if len(query) == 0 {
		return 0, true
	}
	t := []rune(strings.ToLower(target))
	score := 0
	ti := 0
	prev := -2
	for _, qr := range query {
		found := false
		for ti < len(t) {
			if t[ti] != qr {
				ti++
				continue
			}
			score++
			if ti == prev+1 {
				score += 4
			}
			if ti == 0 || t[ti-1] == ' ' || t[ti-1] == '-' {
				score += 2
			}
			prev = ti
			ti++
			found = true
			break
		}
		if !found {
			return 0, false
		}
	}
	return score, true
}
