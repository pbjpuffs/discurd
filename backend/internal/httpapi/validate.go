package httpapi

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	usernameRe    = regexp.MustCompile(`^[a-zA-Z0-9_]{2,32}$`)
	emailRe       = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	accentColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// effectTypes is the closed set of broadcast effect names (§3, FEATURES-v2).
var effectTypes = map[string]bool{
	"lightning": true,
	"confetti":  true,
	"hearts":    true,
	"snow":      true,
	"rain":      true,
}

// ValidateUsername enforces 2–32 chars of [a-zA-Z0-9_].
func ValidateUsername(u string) error {
	if !usernameRe.MatchString(u) {
		return fmt.Errorf("username must be 2-32 characters of letters, digits, or underscore")
	}
	return nil
}

// NormalizeEmail lowercases and trims an email, validating its shape.
func NormalizeEmail(e string) (string, error) {
	e = strings.ToLower(strings.TrimSpace(e))
	if len(e) > 254 || !emailRe.MatchString(e) {
		return "", fmt.Errorf("invalid email address")
	}
	return e, nil
}

// ValidatePassword enforces the ≥ 8 chars rule.
func ValidatePassword(p string) error {
	if len(p) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if len(p) > 512 {
		return fmt.Errorf("password too long")
	}
	return nil
}

// ValidateGuildName enforces 2–100 chars.
func ValidateGuildName(n string) error {
	n = strings.TrimSpace(n)
	if l := len([]rune(n)); l < 2 || l > 100 {
		return fmt.Errorf("guild name must be 2-100 characters")
	}
	return nil
}

// SanitizeChannelName lowercases, converts whitespace runs to '-', drops
// anything outside [a-z0-9_-], and collapses repeated dashes. Returns an
// error if nothing valid remains or the input exceeds 100 chars.
func SanitizeChannelName(n string) (string, error) {
	n = strings.TrimSpace(n)
	if l := len([]rune(n)); l < 1 || l > 100 {
		return "", fmt.Errorf("channel name must be 1-100 characters")
	}
	n = strings.ToLower(n)
	var b strings.Builder
	lastDash := false
	for _, r := range n {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
			lastDash = false
		case r == ' ', r == '\t', r == '-':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "", fmt.Errorf("channel name has no valid characters")
	}
	return out, nil
}

// ValidateChannelType normalizes a channel type. An empty value defaults to
// "text"; "text" and "voice" (case-insensitive, trimmed) are accepted;
// anything else is rejected (§9.5).
func ValidateChannelType(t string) (string, error) {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "":
		return "text", nil
	case "text", "voice":
		return t, nil
	default:
		return "", fmt.Errorf("channel type must be \"text\" or \"voice\"")
	}
}

// ValidateTopic enforces topic ≤ 1024 chars.
func ValidateTopic(t string) error {
	if len([]rune(t)) > 1024 {
		return fmt.Errorf("topic must be at most 1024 characters")
	}
	return nil
}

// ValidateMessageContent enforces 1–4000 chars, allowing empty content only
// when the message carries attachments.
func ValidateMessageContent(content string, hasAttachments bool) error {
	l := len([]rune(content))
	if l == 0 && !hasAttachments {
		return fmt.Errorf("message content must not be empty")
	}
	if l > 4000 {
		return fmt.Errorf("message content must be at most 4000 characters")
	}
	return nil
}

// ValidateBio enforces bio ≤ 190 characters.
func ValidateBio(b string) error {
	if len([]rune(b)) > 190 {
		return fmt.Errorf("bio must be at most 190 characters")
	}
	return nil
}

// ValidatePronouns enforces pronouns ≤ 40 characters.
func ValidatePronouns(p string) error {
	if len([]rune(p)) > 40 {
		return fmt.Errorf("pronouns must be at most 40 characters")
	}
	return nil
}

// ValidateAccentColor accepts an empty value or a #RRGGBB hex color.
func ValidateAccentColor(c string) error {
	if c == "" {
		return nil
	}
	if !accentColorRe.MatchString(c) {
		return fmt.Errorf("accent_color must be a #RRGGBB hex color or empty")
	}
	return nil
}

// ValidateEmoji enforces a 1–32 rune reaction key that is not whitespace-only.
func ValidateEmoji(e string) error {
	if strings.TrimSpace(e) == "" {
		return fmt.Errorf("emoji must not be blank")
	}
	if l := len([]rune(e)); l < 1 || l > 32 {
		return fmt.Errorf("emoji must be 1-32 characters")
	}
	return nil
}

// ValidateEffectType enforces the closed effect-type enum.
func ValidateEffectType(t string) error {
	if !effectTypes[t] {
		return fmt.Errorf("effect type must be one of lightning, confetti, hearts, snow, rain")
	}
	return nil
}

// SanitizeFilename strips path components and any character outside
// [a-zA-Z0-9._-], guaranteeing a non-empty, bounded result.
func SanitizeFilename(name string) string {
	// Strip both unix and windows path separators.
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		out = "file"
	}
	if len(out) > 128 {
		out = out[len(out)-128:]
	}
	return out
}
