package httpapi

import (
	"testing"

	"github.com/gocql/gocql"

	"discurd/internal/models"
	"discurd/internal/store"
)

func TestAggregateReactions(t *testing.T) {
	me := gocql.TimeUUID()
	other := gocql.TimeUUID()

	// Rows arrive clustered by (emoji, user_id); aggregation preserves the
	// first-seen emoji order and counts per emoji.
	rows := []store.ReactionRow{
		{Emoji: "🔥", UserID: other},
		{Emoji: "🔥", UserID: me},
		{Emoji: "👍", UserID: other},
	}
	got := aggregateReactions(rows, me)
	want := []models.Reaction{
		{Emoji: "🔥", Count: 2, Me: true},
		{Emoji: "👍", Count: 1, Me: false},
	}
	if len(got) != len(want) {
		t.Fatalf("aggregateReactions len = %d, want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reaction[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// No rows must yield a non-nil empty slice (JSON [] not null).
	if empty := aggregateReactions(nil, me); empty == nil || len(empty) != 0 {
		t.Errorf("aggregateReactions(nil) = %+v, want non-nil empty", empty)
	}
}

func TestIsAllowedGifURL(t *testing.T) {
	allowed := []string{
		"https://media.tenor.com/abc/x.gif",
		"https://c.tenor.com/abc/x.gif",
		"https://media1.tenor.com/abc/x.gif",
		"https://media.giphy.com/media/x/giphy.gif",
		"https://i.giphy.com/x.gif",
	}
	for _, u := range allowed {
		if !isAllowedGifURL(u) {
			t.Errorf("isAllowedGifURL(%q) = false, want true", u)
		}
	}
	denied := []string{
		"http://media.tenor.com/x.gif", // not https
		"https://evil.com/x.gif",
		"https://nottenor.com/x.gif", // apex lookalike, no dot boundary
		"https://tenor.com.evil.com/x.gif",
		"https://media.tenor.com.evil.com/x.gif",
		"javascript:alert(1)",
		"",
	}
	for _, u := range denied {
		if isAllowedGifURL(u) {
			t.Errorf("isAllowedGifURL(%q) = true, want false", u)
		}
	}
}

func TestValidateAttachmentsGIF(t *testing.T) {
	chID := "11111111-1111-1111-1111-111111111111"

	// Local upload under this channel's prefix: allowed.
	if err := validateAttachments(chID, []models.Attachment{
		{URL: "/files/attachments/" + chID + "/pic.png", ContentType: "image/png"},
	}); err != nil {
		t.Errorf("local attachment rejected: %v", err)
	}

	// Tenor GIF with image/gif content type: allowed.
	if err := validateAttachments(chID, []models.Attachment{
		{URL: "https://media.tenor.com/x/y.gif", ContentType: "image/gif", Filename: "gif"},
	}); err != nil {
		t.Errorf("tenor gif attachment rejected: %v", err)
	}

	// External GIF host but wrong content type: rejected.
	if err := validateAttachments(chID, []models.Attachment{
		{URL: "https://media.tenor.com/x/y.gif", ContentType: "image/png"},
	}); err == nil {
		t.Error("tenor url with non-gif content type accepted")
	}

	// image/gif on a non-allowlisted host: rejected.
	if err := validateAttachments(chID, []models.Attachment{
		{URL: "https://evil.com/x.gif", ContentType: "image/gif"},
	}); err == nil {
		t.Error("gif on disallowed host accepted")
	}
}
