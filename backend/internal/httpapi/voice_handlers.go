package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/livekit/protocol/auth"
)

// voiceTokenTTL bounds the lifetime of a minted LiveKit join token. The token
// only authorizes joining the room; media flows peer-to-SFU once connected.
const voiceTokenTTL = 6 * time.Hour

// handleVoiceToken mints a short-lived LiveKit join token for a voice channel
// (§9.5). The room name is the channel id and the participant identity is the
// user id. 400 if the channel is not a voice channel.
func (s *Server) handleVoiceToken(w http.ResponseWriter, r *http.Request) {
	channel, _, uid, ok := s.channelContext(w, r)
	if !ok {
		return
	}
	if channel.Model().Type != "voice" {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, "not a voice channel")
		return
	}

	user, err := s.users.GetByID(r.Context(), uid)
	if err != nil {
		writeStoreError(w, s.logger, err, "user not found")
		return
	}

	metadata, err := json.Marshal(map[string]string{"avatar_url": user.AvatarURL})
	if err != nil {
		s.logger.Error("marshal voice token metadata", "error", err.Error())
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}

	room := channel.ID.String()
	grant := &auth.VideoGrant{RoomJoin: true, Room: room}
	grant.SetCanPublish(true)
	grant.SetCanSubscribe(true)
	grant.SetCanPublishData(true)

	token := auth.NewAccessToken(s.cfg.LiveKitAPIKey, s.cfg.LiveKitAPISecret).
		SetVideoGrant(grant).
		SetIdentity(uid.String()).
		SetName(user.Username).
		SetMetadata(string(metadata)).
		SetValidFor(voiceTokenTTL)

	jwt, err := token.ToJWT()
	if err != nil {
		s.logger.Error("mint voice token", "error", err.Error())
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"url":   s.cfg.LiveKitWSURL,
		"token": jwt,
		"room":  room,
	})
}
