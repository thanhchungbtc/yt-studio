package entity

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidChannel is returned by the Channel constructor for invalid input.
var ErrInvalidChannel = errors.New("invalid channel")

// CredentialStatus describes whether a channel can currently upload.
type CredentialStatus string

// The complete set of credential states.
const (
	CredentialStatusMissing CredentialStatus = "missing"
	CredentialStatusValid   CredentialStatus = "valid"
	CredentialStatusExpired CredentialStatus = "expired"
)

// AllCredentialStatuses lists every CredentialStatus, for validation and the
// UI.
var AllCredentialStatuses = []CredentialStatus{
	CredentialStatusMissing,
	CredentialStatusValid,
	CredentialStatusExpired,
}

// Valid reports whether the status is one of the known constants.
func (c CredentialStatus) Valid() bool {
	switch c {
	case CredentialStatusMissing, CredentialStatusValid, CredentialStatusExpired:
		return true
	default:
		return false
	}
}

// StyleConfig is the per-channel creative configuration handed to providers.
type StyleConfig struct {
	// Tone steers the LLM's narration voice, e.g. "calm, measured, nocturnal".
	Tone string
	// Voice names the TTS voice for this channel.
	Voice string
	// ImageStyle steers the image provider, e.g. "muted watercolour, wide shot".
	ImageStyle string
	// Language is a BCP-47 tag; scripts and metadata are produced in it.
	Language string
	// WordsPerChapter is the target narration length of one chapter.
	WordsPerChapter int
	// WordsPerMinute is how fast this channel's voice reads. It is what turns a
	// word count into a duration, in both directions: the blueprint budgets
	// words from a target length, and a finished script reports the length it
	// actually came to.
	WordsPerMinute int
}

// Channel owns identity, creative configuration and upload credentials.
type Channel struct {
	ID          ChannelID
	Slug        Slug
	Name        string
	Description string
	Style       StyleConfig
	Credentials CredentialStatus
	// VideoSeq is the per-channel counter behind video refs (DSS-1, DSS-2...).
	VideoSeq  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewChannel validates and constructs a Channel. Cross-field rules that a
// struct tag cannot express live here.
func NewChannel(id ChannelID, slug Slug, name string, style StyleConfig, now time.Time) (Channel, error) {
	if strings.TrimSpace(string(id)) == "" {
		return Channel{}, fmt.Errorf("%w: id must not be empty", ErrInvalidChannel)
	}
	if _, err := NewSlug(string(slug)); err != nil {
		return Channel{}, fmt.Errorf("%w: %w", ErrInvalidChannel, err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Channel{}, fmt.Errorf("%w: name must not be empty", ErrInvalidChannel)
	}
	if style.WordsPerChapter <= 0 {
		style.WordsPerChapter = DefaultWordsPerChapter
	}
	if style.WordsPerMinute <= 0 {
		style.WordsPerMinute = DefaultWordsPerMinute
	}
	if style.Language == "" {
		style.Language = DefaultLanguage
	}
	return Channel{
		ID:          id,
		Slug:        slug,
		Name:        name,
		Style:       style,
		Credentials: CredentialStatusMissing,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Defaults applied by the Channel constructor when a field is left blank.
const (
	DefaultWordsPerChapter = 450
	DefaultLanguage        = "en-US"
	// DefaultWordsPerMinute is an unhurried narration speed, chosen for a
	// channel someone falls asleep to rather than for a briefing.
	DefaultWordsPerMinute = 130
)
