package entity

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ErrInvalidSlug is returned by NewSlug for any input that is not lowercase
// kebab-case within the length bounds.
var ErrInvalidSlug = errors.New("invalid slug")

// ErrInvalidRef is returned by NewRef and ParseRef for malformed video refs.
var ErrInvalidRef = errors.New("invalid ref")

const (
	slugMinLen = 2
	slugMaxLen = 64
)

// Slug is the stable, human-readable natural key of a Channel: lowercase
// kebab-case, unique, and immutable once chosen (§3). It is a domain type, not
// a bare string, so validation happens exactly once at the constructor.
type Slug string

// String returns the underlying text of the slug.
func (s Slug) String() string { return string(s) }

// NewSlug validates and constructs a Slug.
//
// Valid shape: lowercase ASCII letters and digits, separated by single hyphens,
// starting and ending with an alphanumeric. `deep-sleep-stories` is valid;
// `Deep_Sleep`, `-lead`, `trail-` and `double--hyphen` are not.
func NewSlug(s string) (Slug, error) {
	if len(s) < slugMinLen || len(s) > slugMaxLen {
		return "", fmt.Errorf("%w %q: length must be %d..%d", ErrInvalidSlug, s, slugMinLen, slugMaxLen)
	}
	prevHyphen := false
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			prevHyphen = false
		case r == '-':
			if i == 0 || i == len(s)-1 {
				return "", fmt.Errorf("%w %q: must not start or end with a hyphen", ErrInvalidSlug, s)
			}
			if prevHyphen {
				return "", fmt.Errorf("%w %q: must not contain consecutive hyphens", ErrInvalidSlug, s)
			}
			prevHyphen = true
		default:
			return "", fmt.Errorf("%w %q: only lowercase letters, digits and hyphens are allowed", ErrInvalidSlug, s)
		}
	}
	return Slug(s), nil
}

// SlugifyName derives a candidate slug from a human display name. It is a
// convenience for callers that did not supply an explicit slug; the result is
// still validated by NewSlug.
func SlugifyName(name string) (Slug, error) {
	var b strings.Builder
	b.Grow(len(name))
	prevHyphen := true // suppresses a leading hyphen
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) && r < unicode.MaxASCII, unicode.IsDigit(r) && r < unicode.MaxASCII:
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return NewSlug(strings.Trim(b.String(), "-"))
}

// Prefix returns the uppercase issue-key prefix derived from a slug:
// `deep-sleep-stories` yields `DSS`, `history` yields `HIS`. It is the first
// letter of each hyphenated word, padded from the first word when there are
// fewer than three words.
func (s Slug) Prefix() string {
	words := strings.Split(string(s), "-")
	var b strings.Builder
	b.Grow(4)
	for _, w := range words {
		if w == "" {
			continue
		}
		b.WriteByte(byte(unicode.ToUpper(rune(w[0]))))
		if b.Len() == 3 {
			return b.String()
		}
	}
	// Fewer than three words: pad from the first word's remaining letters.
	first := words[0]
	for i := 1; i < len(first) && b.Len() < 3; i++ {
		b.WriteByte(byte(unicode.ToUpper(rune(first[i]))))
	}
	for b.Len() < 3 {
		b.WriteByte('X')
	}
	return b.String()
}

// Ref is the stable, human-readable natural key of a Video, shaped like a JIRA
// issue key: `DSS-1`, `HIS-42` (§3). The prefix comes from the owning channel's
// slug and the number is a per-channel counter.
type Ref string

// String returns the underlying text of the ref.
func (r Ref) String() string { return string(r) }

// NewRef builds a video ref from a channel slug and a per-channel sequence
// number.
func NewRef(channel Slug, seq int) (Ref, error) {
	if seq < 1 {
		return "", fmt.Errorf("%w: sequence must be >= 1, got %d", ErrInvalidRef, seq)
	}
	if _, err := NewSlug(string(channel)); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidRef, err)
	}
	var b strings.Builder
	b.Grow(8)
	b.WriteString(channel.Prefix())
	b.WriteByte('-')
	b.WriteString(strconv.Itoa(seq))
	return Ref(b.String()), nil
}

// ParseRef validates the shape of a ref and returns its prefix and sequence.
func ParseRef(s string) (prefix string, seq int, err error) {
	i := strings.LastIndexByte(s, '-')
	if i <= 0 || i == len(s)-1 {
		return "", 0, fmt.Errorf("%w %q: expected PREFIX-N", ErrInvalidRef, s)
	}
	prefix = s[:i]
	for _, r := range prefix {
		if r < 'A' || r > 'Z' {
			return "", 0, fmt.Errorf("%w %q: prefix must be uppercase letters", ErrInvalidRef, s)
		}
	}
	seq, err = strconv.Atoi(s[i+1:])
	if err != nil || seq < 1 {
		return "", 0, fmt.Errorf("%w %q: sequence must be a positive integer", ErrInvalidRef, s)
	}
	return prefix, seq, nil
}

// LooksLikeRef reports whether s has the shape of a video ref rather than an
// opaque id. It is used by the API to resolve `/api/videos/{refOrID}` (§3).
func LooksLikeRef(s string) bool {
	_, _, err := ParseRef(s)
	return err == nil
}

// LooksLikeSlug reports whether s has the shape of a channel slug rather than
// an opaque id.
func LooksLikeSlug(s string) bool {
	_, err := NewSlug(s)
	return err == nil
}
