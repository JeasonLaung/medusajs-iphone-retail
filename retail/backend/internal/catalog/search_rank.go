package catalog

import (
	"sort"
	"strings"
)

type searchOccurrence struct {
	term     string
	position int
	weight   int
}

// The installed source provider concatenates title(A), description(D), then
// option_values(B). Each field offset is its last retained lexeme position,
// so trailing stopwords do not separate fields. Parsing retains up to 255
// positions per lexeme; concatenation allows 256. Positions saturate at 16383.
func searchVector(h StoreSearchHit) []searchOccurrence {
	fields := []string{h.Title, h.description, strings.Join(h.Values["option_values"], " ")}
	weights := []int{3, 0, 2}
	byTerm := map[string][]searchOccurrence{}
	offset := 0
	for f, text := range fields {
		local := map[string][]int{}
		for _, word := range searchLexemes(text) {
			position := min(word.Position, 16383)
			positions := local[word.Term]
			if len(positions) < 255 && (len(positions) == 0 || positions[len(positions)-1] != position) {
				local[word.Term] = append(positions, position)
			}
		}
		for term, positions := range local {
			for _, position := range positions {
				position = min(position+offset, 16383)
				entries := byTerm[term]
				if len(entries) > 0 && entries[len(entries)-1].position == position {
					entries[len(entries)-1].weight = max(entries[len(entries)-1].weight, weights[f])
				} else if len(entries) < 256 {
					entries = append(entries, searchOccurrence{term, position, weights[f]})
				}
				byTerm[term] = entries
			}
		}
		// A full lexeme's discarded incoming positions cannot move the next
		// field offset: concatenation uses the resulting stored vector.
		offset = 0
		for _, entries := range byTerm {
			if len(entries) > 0 {
				offset = max(offset, entries[len(entries)-1].position)
			}
		}
	}
	vector := []searchOccurrence{}
	for _, entries := range byTerm {
		vector = append(vector, entries...)
	}
	sort.Slice(vector, func(i, j int) bool {
		a, b := vector[i], vector[j]
		if a.position != b.position {
			return a.position < b.position
		}
		if a.weight != b.weight {
			return a.weight < b.weight
		}
		return a.term < b.term
	})
	return vector
}

// Source uses ts_rank_cd with default weights/normalization, AND operands and
// optionally a prefix on the final operand. A document occurrence can satisfy
// several operands (including a repeated term), but contributes its weight once.
// Compute the minimal covers in occurrence order, then penalize positional gaps.
// Algorithm reference: PostgreSQL REL_16_STABLE src/backend/utils/adt/tsrank.c.
func searchCoverRank(vector []searchOccurrence, terms []string, prefix bool) (float32, bool) {
	type entry struct {
		searchOccurrence
		operands []int
	}
	doc := []entry{}
	matched := make([]bool, len(terms))
	for _, word := range vector {
		operands := []int{}
		for i, term := range terms {
			if word.term == term || prefix && i == len(terms)-1 && strings.HasPrefix(word.term, term) {
				operands = append(operands, i)
				matched[i] = true
			}
		}
		if len(operands) > 0 {
			doc = append(doc, entry{word, operands})
		}
	}
	for _, found := range matched {
		if !found {
			return 0, false
		}
	}
	weights := []float32{0.1, 0.2, 0.4, 1}
	score := float64(0)
	for start := 0; start < len(doc); {
		seen := make([]bool, len(terms))
		remaining := len(terms)
		add := func(e entry) {
			for _, operand := range e.operands {
				if !seen[operand] {
					seen[operand] = true
					remaining--
				}
			}
		}
		end := start
		for ; end < len(doc); end++ {
			add(doc[end])
			if remaining == 0 {
				break
			}
		}
		if end == len(doc) {
			break
		}
		clear(seen)
		remaining = len(terms)
		begin := end
		for ; begin >= start; begin-- {
			add(doc[begin])
			if remaining == 0 {
				break
			}
		}
		inverse := float64(0)
		for _, e := range doc[begin : end+1] {
			inverse += 1 / float64(weights[e.weight])
		}
		noise := doc[end].position - doc[begin].position - (end - begin)
		if noise < 0 {
			noise = (end - begin) / 2
		}
		score += float64(end-begin+1) / inverse / float64(1+noise)
		start = begin + 1
	}
	return float32(score), true
}
