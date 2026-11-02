// Package ids holds the PS5 identifier rules: account ids, online ids, npIds.
package ids

import (
	"errors"
	"strconv"
	"strings"
)

// Online id rules: 3 to 16 characters, letters, digits, hyphen and underscore,
// starting with a letter.
const (
	OnlineIDMin = 3
	OnlineIDMax = 16
)

// NewAccountID is the decimal string form of a 64-bit PSN account id.
func NewAccountID(n uint64) string { return strconv.FormatUint(n, 10) }

// ParseAccountID checks s is a decimal uint64 without sign or leading zeros
// (except "0" itself) and returns it.
func ParseAccountID(s string) (uint64, error) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, errors.New("invalid account id")
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, errors.New("invalid account id")
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, errors.New("invalid account id")
	}
	return n, nil
}

// ValidateOnlineID reports whether s is an acceptable online id.
func ValidateOnlineID(s string) error {
	if len(s) < OnlineIDMin || len(s) > OnlineIDMax {
		return errors.New("online id must be 3 to 16 characters")
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if i == 0 && !letter {
			return errors.New("online id must start with a letter")
		}
		if !letter && !(c >= '0' && c <= '9') && c != '-' && c != '_' {
			return errors.New("online id may only contain letters, digits, - and _")
		}
	}
	return nil
}

// NormalizeOnlineID is the case-folded lookup form of an online id.
func NormalizeOnlineID(s string) string { return strings.ToLower(s) }

// NpID is an npId: an online id on a service domain.
type NpID struct {
	OnlineID string
	Domain   string // e.g. "np"
}

// DefaultDomain is the domain OpenPak puts in npIds.
const DefaultDomain = "np"

// String renders "onlineId@domain".
func (n NpID) String() string { return n.OnlineID + "@" + n.Domain }

// ParseNpID parses "onlineId@domain" and validates the online id.
func ParseNpID(s string) (NpID, error) {
	id, dom, ok := strings.Cut(s, "@")
	if !ok || dom == "" || strings.Contains(dom, "@") {
		return NpID{}, errors.New("invalid npId")
	}
	if err := ValidateOnlineID(id); err != nil {
		return NpID{}, err
	}
	return NpID{OnlineID: id, Domain: dom}, nil
}
