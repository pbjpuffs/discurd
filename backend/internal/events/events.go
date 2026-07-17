// Package events implements the NATS event envelope and publish/subscribe
// helpers per docs/ARCHITECTURE.md §6.
package events

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

// Event type strings — the contract vocabulary.
const (
	TypeMessageCreate         = "MESSAGE_CREATE"
	TypeMessageUpdate         = "MESSAGE_UPDATE"
	TypeMessageDelete         = "MESSAGE_DELETE"
	TypeMessageReactionAdd    = "MESSAGE_REACTION_ADD"
	TypeMessageReactionRemove = "MESSAGE_REACTION_REMOVE"
	TypeEffect                = "EFFECT"
	TypeTypingStart           = "TYPING_START"
	TypeChannelCreate         = "CHANNEL_CREATE"
	TypeGuildMemberAdd        = "GUILD_MEMBER_ADD"
	TypePresenceUpdate        = "PRESENCE_UPDATE"
	TypeGuildCreate           = "GUILD_CREATE"
	TypeReady                 = "READY"
)

// SubjectWildcard is what the gateway subscribes to.
const SubjectWildcard = "discurd.events.>"

// Envelope is the JSON payload carried on every event subject.
type Envelope struct {
	T       string          `json:"t"`
	GuildID string          `json:"guild_id"`
	D       json.RawMessage `json:"d"`
}

// GuildSubject returns the subject for guild-scoped events.
func GuildSubject(guildID string) string {
	return fmt.Sprintf("discurd.events.guild.%s", guildID)
}

// UserSubject returns the subject for user-targeted events.
func UserSubject(userID string) string {
	return fmt.Sprintf("discurd.events.user.%s", userID)
}

// Publisher publishes contract events to NATS.
type Publisher struct {
	nc *nats.Conn
}

// NewPublisher wraps an established NATS connection.
func NewPublisher(nc *nats.Conn) *Publisher {
	return &Publisher{nc: nc}
}

func (p *Publisher) publish(subject, eventType, guildID string, payload any) error {
	d, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event %s payload: %w", eventType, err)
	}
	env, err := json.Marshal(Envelope{T: eventType, GuildID: guildID, D: d})
	if err != nil {
		return fmt.Errorf("marshal event %s envelope: %w", eventType, err)
	}
	return p.nc.Publish(subject, env)
}

// ToGuild publishes a guild-scoped event.
func (p *Publisher) ToGuild(guildID, eventType string, payload any) error {
	return p.publish(GuildSubject(guildID), eventType, guildID, payload)
}

// ToUser publishes a user-targeted event. guildID may be empty.
func (p *Publisher) ToUser(userID, eventType, guildID string, payload any) error {
	return p.publish(UserSubject(userID), eventType, guildID, payload)
}
