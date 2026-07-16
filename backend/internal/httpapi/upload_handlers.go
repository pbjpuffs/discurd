package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"discurd/internal/models"
	"discurd/internal/objstore"
)

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// readUpload extracts the multipart "file" part enforcing the size cap, and
// returns the bytes, sanitized filename, and detected content type.
func (s *Server) readUpload(w http.ResponseWriter, r *http.Request) ([]byte, string, string, bool) {
	maxBytes := int64(s.cfg.MaxUploadMB) << 20
	// Allow some slack for multipart framing on top of the file itself.
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))

	if err := r.ParseMultipartForm(maxBytes + (1 << 20)); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, CodeTooLarge, "upload exceeds size limit")
		} else {
			writeError(w, http.StatusBadRequest, CodeValidationFailed, "expected multipart form with a 'file' part")
		}
		return nil, "", "", false
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, "missing 'file' part")
		return nil, "", "", false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, "could not read upload")
		return nil, "", "", false
	}
	if int64(len(data)) > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, CodeTooLarge, "upload exceeds size limit")
		return nil, "", "", false
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, "uploaded file is empty")
		return nil, "", "", false
	}

	return data, SanitizeFilename(header.Filename), sniffContentType(data), true
}

// sniffContentType derives a trustworthy content type from the bytes alone,
// never from the client-declared multipart header (which an attacker fully
// controls). Only genuine image types are returned verbatim; everything else
// collapses to application/octet-stream so a malicious HTML/SVG document can
// never be persisted with — and later served under — an active content type
// on our own origin. Combined with the Content-Disposition:attachment header
// Traefik adds to every /files response, this closes the stored-XSS vector.
func sniffContentType(data []byte) string {
	ct := http.DetectContentType(data)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	return "application/octet-stream"
}

func (s *Server) handleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(w, r)
	if !ok {
		return
	}
	data, filename, contentType, ok := s.readUpload(w, r)
	if !ok {
		return
	}
	if !strings.HasPrefix(contentType, "image/") {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, "avatar must be an image")
		return
	}

	ext := strings.TrimPrefix(path.Ext(filename), ".")
	if ext == "" {
		ext = "png"
	}
	random, err := randomHex(8)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}
	key := user.ID.String() + "/" + random + "." + ext

	url, err := s.objects.Upload(r.Context(), objstore.BucketAvatars, key,
		bytes.NewReader(data), int64(len(data)), contentType)
	if err != nil {
		s.logger.Error("avatar upload failed", "error", err.Error())
		writeError(w, http.StatusInternalServerError, CodeInternal, "upload failed")
		return
	}
	if err := s.users.UpdateAvatar(r.Context(), user.ID, url); err != nil {
		writeStoreError(w, s.logger, err, "user not found")
		return
	}
	s.userCache.Invalidate(user.ID)

	writeJSON(w, http.StatusOK, map[string]string{"avatar_url": url})
}

func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	channel, _, _, ok := s.channelContext(w, r)
	if !ok {
		return
	}
	data, filename, contentType, ok := s.readUpload(w, r)
	if !ok {
		return
	}

	random, err := randomHex(8)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}
	key := channel.ID.String() + "/" + random + "/" + filename

	url, err := s.objects.Upload(r.Context(), objstore.BucketAttachments, key,
		bytes.NewReader(data), int64(len(data)), contentType)
	if err != nil {
		s.logger.Error("attachment upload failed", "error", err.Error())
		writeError(w, http.StatusInternalServerError, CodeInternal, "upload failed")
		return
	}

	writeJSON(w, http.StatusCreated, models.Attachment{
		URL:         url,
		Filename:    filename,
		Size:        int64(len(data)),
		ContentType: contentType,
	})
}
