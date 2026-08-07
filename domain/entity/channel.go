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
// Deliberately empty for now; the seam is kept so the fields can return.
type StyleConfig struct{}

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

// NewChannel validates and constructs a Channel.
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
