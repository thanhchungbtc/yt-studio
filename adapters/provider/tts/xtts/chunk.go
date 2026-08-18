package xtts

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The text chunking, ported one for one from the Python this backend replaces.
// It lives with the client rather than with the shared audio helpers because
// only this backend chunks: a chapter is split because XTTS degrades on long
// inputs, which is a fact about one model and not about narration.
//
// Lengths are in runes throughout: the Python counted code points, and bytes
// would change the chunking of any script with a curly apostrophe.

// sentenceTerminators is the set a sentence may end on. The CJK marks are
// deliberate: splitting a Chinese paragraph on ASCII periods yields one chunk.
const sentenceTerminators = ".!?。！？"

// splitSentences cuts text into sentences, keeping the terminal punctuation and
// the whitespace that follows it, so joining the result reproduces the input
// exactly. A trailing fragment with no terminator is a sentence too.
func splitSentences(text string) []string {
	if text == "" {
		return nil
	}
	sentences := make([]string, 0, 8)
	start, i := 0, 0
	for i < len(text) {
		r, size := utf8.DecodeRuneInString(text[i:])
		i += size
		if !strings.ContainsRune(sentenceTerminators, r) {
			continue
		}
		// "!?" and "..." end one sentence, not three, so the whole run goes with
		// it — and so does the space after, which keeps rejoining lossless.
		i += runLength(text[i:], func(r rune) bool { return strings.ContainsRune(sentenceTerminators, r) })
		i += runLength(text[i:], unicode.IsSpace)

		sentences = append(sentences, text[start:i])
		start = i
	}
	if start < len(text) {
		sentences = append(sentences, text[start:])
	}
	return sentences
}

// runLength reports the byte length of the leading run of runes satisfying fn.
func runLength(s string, fn func(rune) bool) int {
	n := 0
	for n < len(s) {
		r, size := utf8.DecodeRuneInString(s[n:])
		if !fn(r) {
			break
		}
		n += size
	}
	return n
}

// chunkTextBySentence splits text into chunks of roughly equal size, none below
// minChars where the boundaries allow. The count is fixed first, so each chunk
// targets len(text)/n; sentences accumulate until closing lands nearer that
// target than adding would. A sentence longer than the target is never broken.
func chunkTextBySentence(text string, minChars int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	// The Python divided unguarded and would have raised. A misconfigured floor
	// should cost the chunking, not the chapter.
	if minChars <= 0 {
		return []string{text}
	}
	total := utf8.RuneCountInString(text)
	chunkCount := max(1, total/minChars)
	if chunkCount == 1 {
		return []string{text}
	}
	sentences := splitSentences(text)
	if len(sentences) <= 1 {
		return []string{text}
	}

	chunks := make([]string, 0, chunkCount)
	var current strings.Builder
	currentLen := 0
	remainingChars := total
	remainingChunks := chunkCount

	for _, sentence := range sentences {
		sentenceLen := utf8.RuneCountInString(sentence)
		if currentLen > 0 && remainingChunks > 1 {
			target := float64(remainingChars) / float64(remainingChunks)
			before := math.Abs(float64(currentLen) - target)
			after := math.Abs(float64(currentLen+sentenceLen) - target)
			if before < after {
				chunks = append(chunks, strings.TrimSpace(current.String()))
				// The untrimmed length is what was spent, so the remaining chunks
				// size themselves against what is genuinely left.
				remainingChars -= currentLen
				remainingChunks--
				current.Reset()
				currentLen = 0
			}
		}
		current.WriteString(sentence)
		currentLen += sentenceLen
	}
	if last := strings.TrimSpace(current.String()); last != "" {
		chunks = append(chunks, last)
	}
	return chunks
}
