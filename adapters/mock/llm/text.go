package llm

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Deterministic prose generation. Every phrase is chosen by a PRNG seeded only
// from the request, so the same inputs always produce byte-identical output and
// therefore the same content address.

var (
	titleOpeners = []string{
		"The Long Night of the", "Before the Storm at the", "What Remained of the",
		"The Quiet Years of the", "Crossing the", "The Making of the",
		"A Winter on the", "The Last Days of the", "Notes From the",
		"The Turning at the", "Under the", "The Second",
	}
	titleSubjects = []string{
		"Harbour", "Northern Road", "Copper Valley", "Grey Coast",
		"Old Bridge", "Salt Flats", "Marble Quarry", "River Bend",
		"Lantern District", "High Pass", "Amber Court", "Silent Mill",
	}
	summaryVerbs = []string{
		"traces", "follows", "settles into", "opens on", "returns to", "widens out from",
	}
	summaryObjects = []string{
		"a single decision and everything it displaced",
		"the people who stayed after everyone else had gone",
		"a map that stopped matching the ground beneath it",
		"one winter told through the things that survived it",
		"a correspondence that outlasted both correspondents",
		"the slow arithmetic of a place running out of time",
	}
	narrationOpeners = []string{
		"There is a particular quality to", "It began, as these things do, with",
		"Consider for a moment", "Long before anyone thought to write it down,",
		"The record is thin here, which is itself worth noticing about",
		"What follows is best understood as",
	}
	narrationConnectors = []string{
		"And yet", "In the years that followed", "By any measure", "Even so",
		"For a time", "Which is to say", "Understandably", "Almost without exception",
		"More quietly", "In practice", "Against expectation", "Slowly at first",
	}
	narrationNouns = []string{
		"the harbour", "the road north", "the old quarter", "the winter market",
		"the copper works", "the river", "the border villages", "the archive",
		"the shipping ledgers", "the family that stayed", "the second bridge",
		"the surveyor's notes", "the long field", "the lantern makers",
	}
	narrationVerbs = []string{
		"held", "gave way", "changed hands", "was rebuilt", "went quiet",
		"drew people in", "emptied out", "was measured again", "found its shape",
		"outlasted the argument about it", "returned to what it had been",
	}
	narrationTails = []string{
		"and nobody thought to mark the date",
		"which explains rather more than it seems to",
		"though the reasons were never written down",
		"in a way that would matter only much later",
		"for reasons that were entirely practical at the time",
		"and the ledgers show it plainly enough",
		"long after the decision had stopped feeling like one",
	}
	slideSubjects = []string{
		"a wide harbour at low tide", "a stone bridge in thin fog",
		"a lantern-lit street after rain", "an empty market square at dawn",
		"a river bend seen from a ridge", "a shuttered mill against pale sky",
		"a mountain pass under first snow", "a courtyard with a single tree",
	}
	imageLighting = []string{
		"low golden light", "overcast diffuse light", "blue hour",
		"raking late-afternoon sun", "soft moonlight", "flat winter daylight",
	}
	imageComposition = []string{
		"wide establishing shot", "low horizon, tall sky", "centred symmetrical framing",
		"three-quarter view", "high vantage looking down", "shallow foreground detail",
	}
	// Icon subjects are single objects, drawn small: what survives being reduced
	// to a 2 cm tile of line art. The style clause is not here — the server
	// appends it, so restyling the grid never re-rolls these words.
	iconSubjects = []string{
		"a ship's lantern", "a folded paper map", "a stone archway", "a pocket watch",
		"a pair of oars", "a stack of ledgers", "a weather vane", "a hand holding a key",
		"an open gate", "a coil of rope", "a bell in a tower", "a compass rose",
		"a bundle of letters", "a lighthouse", "a wooden crate", "a broken chain",
	}
	iconViews = []string{"side view", "front view", "three-quarter view", "top-down view"}
	// Words a caption is better off without: two words is the whole budget, and
	// none of these earn one.
	captionStopWords = map[string]struct{}{
		"the": {}, "of": {}, "a": {}, "an": {}, "at": {}, "in": {}, "on": {}, "to": {},
		"from": {}, "and": {}, "for": {}, "with": {}, "what": {}, "which": {},
		"before": {}, "under": {}, "after": {}, "second": {},
	}
	tagWords = []string{
		"long form", "narrated", "history", "sleep", "documentary", "storytelling",
		"ambient", "chapters", "deep dive", "relaxing",
	}
)

func chapterTitle(r *rand.Rand, topic string, ordinal int) string {
	var b strings.Builder
	b.Grow(48)
	b.WriteString(pick(r, titleOpeners))
	b.WriteByte(' ')
	b.WriteString(pick(r, titleSubjects))
	if topic != "" && r.IntN(4) == 0 {
		b.WriteString(": ")
		b.WriteString(topic)
	}
	b.WriteString(" (")
	b.WriteString(strconv.Itoa(ordinal))
	b.WriteByte(')')
	return b.String()
}

func chapterSummary(r *rand.Rand, topic string, ordinal int) string {
	var b strings.Builder
	b.Grow(96)
	b.WriteString("Chapter ")
	b.WriteString(strconv.Itoa(ordinal))
	b.WriteByte(' ')
	b.WriteString(pick(r, summaryVerbs))
	b.WriteByte(' ')
	b.WriteString(pick(r, summaryObjects))
	if topic != "" {
		b.WriteString(", set against ")
		b.WriteString(topic)
	}
	b.WriteByte('.')
	return b.String()
}

// narration builds a script of roughly wordTarget words in sentences of varied
// length, using a strings.Builder with reserved capacity rather than repeated
// concatenation.
func narration(seed uint64, title, summary string, wordTarget int) string {
	r := deterministic(seed)
	var b strings.Builder
	b.Grow(wordTarget * 7)

	// The running word count is tracked as text is appended; re-scanning the
	// buffer each iteration would make this quadratic.
	words := 0
	write := func(s string) {
		b.WriteString(s)
		words += countWords(s)
	}

	write(pick(r, narrationOpeners))
	b.WriteByte(' ')
	write(strings.ToLower(strings.TrimSuffix(title, ".")))
	b.WriteString(". ")
	if summary != "" {
		write(summary)
		b.WriteByte(' ')
	}

	for words < wordTarget {
		write(pick(r, narrationConnectors))
		b.WriteByte(' ')
		write(pick(r, narrationNouns))
		b.WriteByte(' ')
		write(pick(r, narrationVerbs))
		if r.IntN(2) == 0 {
			b.WriteString(", ")
			write(pick(r, narrationTails))
		}
		b.WriteString(". ")
	}
	return strings.TrimSpace(b.String())
}

// countWords is a fast field count that does not allocate a slice.
func countWords(s string) int {
	n, inWord := 0, false
	for i := range len(s) {
		switch s[i] {
		case ' ', '\n', '\t', '\r':
			inWord = false
		default:
			if !inWord {
				n++
				inWord = true
			}
		}
	}
	return n
}

func slidePrompt(seed uint64, chapterTitle, chapterSummary string) string {
	r := deterministic(seed)
	var b strings.Builder
	b.Grow(192)
	b.WriteString(pick(r, slideSubjects))
	b.WriteString(", ")
	b.WriteString(pick(r, imageLighting))
	b.WriteString(", ")
	b.WriteString(pick(r, imageComposition))
	b.WriteString(" — for \"")
	b.WriteString(chapterTitle)
	b.WriteString("\"")
	if chapterSummary != "" {
		b.WriteString(": ")
		b.WriteString(chapterSummary)
	}
	return b.String()
}

// iconPrompt describes one tile's subject. It is the subject alone: the shared
// style clause is the server's to append.
func iconPrompt(seed uint64) string {
	r := deterministic(seed)
	return pick(r, iconSubjects) + ", " + pick(r, iconViews)
}

// caption compresses a chapter title into the two words a tile has room for.
//
// Stop words go first, then the trailing ordinal the mock's own titles carry.
// What is left is Title Case, because that is what the reference thumbnails
// use — the headline shouts, the captions label.
func caption(title string) string {
	words := make([]string, 0, 2)
	for _, w := range strings.Fields(title) {
		w = strings.Trim(w, "()[],.:;\"'")
		if w == "" {
			continue
		}
		if _, stop := captionStopWords[strings.ToLower(w)]; stop {
			continue
		}
		if _, err := strconv.Atoi(w); err == nil {
			continue
		}
		words = append(words, titleCase(w))
		if len(words) == 2 {
			break
		}
	}
	if len(words) == 0 {
		return "Untitled"
	}
	return strings.Join(words, " ")
}

func titleCase(w string) string {
	lower := strings.ToLower(w)
	first, size := utf8.DecodeRuneInString(lower)
	if first == utf8.RuneError {
		return lower
	}
	return strings.ToUpper(string(first)) + lower[size:]
}

func tagsFor(r *rand.Rand, topic string) []string {
	tags := make([]string, 0, 6)
	if topic != "" {
		tags = append(tags, strings.ToLower(topic))
	}
	seen := make(map[string]struct{}, 6)
	for _, t := range tags {
		seen[t] = struct{}{}
	}
	for len(tags) < 6 {
		t := pick(r, tagWords)
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		tags = append(tags, t)
	}
	return tags
}
