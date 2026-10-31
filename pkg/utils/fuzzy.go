package utils

import (
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"
)

// FuzzyRank is the indices of texts matching every word of needle, best first.
func FuzzyRank(texts []string, needle string) []int {
	words := strings.Fields(needle)
	scores := make([]int, len(texts))
	matched := make([]int, len(texts))
	literal := make([]int, len(texts))
	for _, word := range words {
		for _, match := range fuzzy.FindNoSort(word, texts) {
			// Words as typed rank by how they appear, then in list order: their
			// fuzzy scores differ by what else the text holds.
			tier := wordTier(texts[match.Index], word)
			matched[match.Index]++
			if tier > 0 {
				scores[match.Index] += 1000 * tier
				literal[match.Index]++
			} else {
				scores[match.Index] += match.Score
			}
		}
	}

	// Scattered matches are only worth showing when nothing has the words
	// as typed.
	counts := matched
	for _, count := range literal {
		if count == len(words) {
			counts = literal
			break
		}
	}

	indices := []int{}
	for index, count := range counts {
		if count == len(words) {
			indices = append(indices, index)
		}
	}
	sort.SliceStable(indices, func(i, j int) bool { return scores[indices[i]] > scores[indices[j]] })

	return indices
}

// wordTier ranks how word appears in text: 3 as a whole word, 2 starting
// one, 1 inside one, 0 only scattered. The fuzzy score alone can't: its
// greedy match takes the "st" of "instances" before the "stop" after it.
func wordTier(text, word string) int {
	text, word = strings.ToLower(text), strings.ToLower(word)
	tier := 0
	for field := range strings.FieldsSeq(text) {
		switch {
		case field == word:
			return 3
		case strings.HasPrefix(field, word):
			tier = 2
		case tier == 0 && strings.Contains(field, word):
			tier = 1
		}
	}

	return tier
}
