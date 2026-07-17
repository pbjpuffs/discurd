package httpapi

import (
	"strings"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	valid := []string{"ab", "alice", "Bob_99", strings.Repeat("x", 32)}
	for _, u := range valid {
		if err := ValidateUsername(u); err != nil {
			t.Errorf("ValidateUsername(%q) unexpectedly failed: %v", u, err)
		}
	}
	invalid := []string{"", "a", strings.Repeat("x", 33), "with space", "dash-ed", "émile", "a@b"}
	for _, u := range invalid {
		if err := ValidateUsername(u); err == nil {
			t.Errorf("ValidateUsername(%q) unexpectedly passed", u)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	got, err := NormalizeEmail("  Alice@Discurd.DEV ")
	if err != nil {
		t.Fatalf("NormalizeEmail: %v", err)
	}
	if got != "alice@discurd.dev" {
		t.Fatalf("NormalizeEmail = %q, want alice@discurd.dev", got)
	}
	for _, e := range []string{"", "nope", "a@b", "two@@at.com", "sp ace@x.com", "@x.com", "a@.com "} {
		if _, err := NormalizeEmail(e); err == nil {
			t.Errorf("NormalizeEmail(%q) unexpectedly passed", e)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("password123"); err != nil {
		t.Errorf("valid password rejected: %v", err)
	}
	if err := ValidatePassword("short"); err == nil {
		t.Error("7-char password accepted")
	}
}

func TestValidateGuildName(t *testing.T) {
	if err := ValidateGuildName("Discurd HQ"); err != nil {
		t.Errorf("valid guild name rejected: %v", err)
	}
	for _, n := range []string{"", "x", strings.Repeat("g", 101)} {
		if err := ValidateGuildName(n); err == nil {
			t.Errorf("ValidateGuildName(%q) unexpectedly passed", n)
		}
	}
	if err := ValidateGuildName(strings.Repeat("g", 100)); err != nil {
		t.Errorf("100-char guild name rejected: %v", err)
	}
}

func TestSanitizeChannelName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"general", "general"},
		{"General", "general"},
		{"Dev Talk", "dev-talk"},
		{"  spaced   out  ", "spaced-out"},
		{"UPPER_case-mix 42", "upper_case-mix-42"},
		{"weird!!chars###here", "weirdcharshere"},
		{"--lead-and-trail--", "lead-and-trail"},
		{"multi - - dash", "multi-dash"},
	}
	for _, tc := range cases {
		got, err := SanitizeChannelName(tc.in)
		if err != nil {
			t.Errorf("SanitizeChannelName(%q) failed: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("SanitizeChannelName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, in := range []string{"", "!!!", "   ", strings.Repeat("c", 101)} {
		if _, err := SanitizeChannelName(in); err == nil {
			t.Errorf("SanitizeChannelName(%q) unexpectedly passed", in)
		}
	}
}

func TestValidateChannelType(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "text"},
		{"text", "text"},
		{"voice", "voice"},
		{"  Voice ", "voice"},
		{"TEXT", "text"},
	}
	for _, tc := range cases {
		got, err := ValidateChannelType(tc.in)
		if err != nil {
			t.Errorf("ValidateChannelType(%q) failed: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ValidateChannelType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, in := range []string{"video", "vc", "text chat", "0", "textvoice"} {
		if _, err := ValidateChannelType(in); err == nil {
			t.Errorf("ValidateChannelType(%q) unexpectedly passed", in)
		}
	}
}

func TestValidateMessageContent(t *testing.T) {
	if err := ValidateMessageContent("hello", false); err != nil {
		t.Errorf("valid content rejected: %v", err)
	}
	if err := ValidateMessageContent("", true); err != nil {
		t.Errorf("empty content with attachments rejected: %v", err)
	}
	if err := ValidateMessageContent("", false); err == nil {
		t.Error("empty content without attachments accepted")
	}
	if err := ValidateMessageContent(strings.Repeat("m", 4000), false); err != nil {
		t.Errorf("4000-char content rejected: %v", err)
	}
	if err := ValidateMessageContent(strings.Repeat("m", 4001), false); err == nil {
		t.Error("4001-char content accepted")
	}
}

func TestValidateBio(t *testing.T) {
	if err := ValidateBio(""); err != nil {
		t.Errorf("empty bio rejected: %v", err)
	}
	if err := ValidateBio(strings.Repeat("b", 190)); err != nil {
		t.Errorf("190-char bio rejected: %v", err)
	}
	if err := ValidateBio(strings.Repeat("b", 191)); err == nil {
		t.Error("191-char bio accepted")
	}
	// Rune-counted, not byte-counted: 190 multibyte runes must pass.
	if err := ValidateBio(strings.Repeat("é", 190)); err != nil {
		t.Errorf("190 multibyte runes rejected: %v", err)
	}
}

func TestValidatePronouns(t *testing.T) {
	if err := ValidatePronouns(""); err != nil {
		t.Errorf("empty pronouns rejected: %v", err)
	}
	if err := ValidatePronouns(strings.Repeat("p", 40)); err != nil {
		t.Errorf("40-char pronouns rejected: %v", err)
	}
	if err := ValidatePronouns(strings.Repeat("p", 41)); err == nil {
		t.Error("41-char pronouns accepted")
	}
}

func TestValidateAccentColor(t *testing.T) {
	for _, c := range []string{"", "#000000", "#FFFFFF", "#a1b2c3", "#A1B2C3"} {
		if err := ValidateAccentColor(c); err != nil {
			t.Errorf("ValidateAccentColor(%q) unexpectedly failed: %v", c, err)
		}
	}
	for _, c := range []string{"000000", "#fff", "#12345", "#1234567", "#gggggg", "red", "#12 456"} {
		if err := ValidateAccentColor(c); err == nil {
			t.Errorf("ValidateAccentColor(%q) unexpectedly passed", c)
		}
	}
}

func TestValidateEmoji(t *testing.T) {
	for _, e := range []string{"🔥", ":fire:", "a", strings.Repeat("x", 32)} {
		if err := ValidateEmoji(e); err != nil {
			t.Errorf("ValidateEmoji(%q) unexpectedly failed: %v", e, err)
		}
	}
	for _, e := range []string{"", " ", "   ", "\t", strings.Repeat("x", 33)} {
		if err := ValidateEmoji(e); err == nil {
			t.Errorf("ValidateEmoji(%q) unexpectedly passed", e)
		}
	}
}

func TestValidateEffectType(t *testing.T) {
	for _, tp := range []string{"lightning", "confetti", "hearts", "snow", "rain"} {
		if err := ValidateEffectType(tp); err != nil {
			t.Errorf("ValidateEffectType(%q) unexpectedly failed: %v", tp, err)
		}
	}
	for _, tp := range []string{"", "Lightning", "thunder", "sparkle", "rain "} {
		if err := ValidateEffectType(tp); err == nil {
			t.Errorf("ValidateEffectType(%q) unexpectedly passed", tp)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"photo.png", "photo.png"},
		{"../../etc/passwd", "passwd"},
		{`C:\Users\bob\cat.jpg`, "cat.jpg"},
		{"my file (1).png", "my_file__1_.png"},
		{"..hidden", "hidden"},
		{"", "file"},
		{"///", "file"},
	}
	for _, tc := range cases {
		if got := SanitizeFilename(tc.in); got != tc.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := SanitizeFilename(strings.Repeat("a", 300) + ".png"); len(got) > 128 {
		t.Errorf("SanitizeFilename did not bound length: %d chars", len(got))
	}
}
