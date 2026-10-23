// Package httpapi exposes the Task domain over HTTP: the webhook endpoints
// and the CRUD of tasks, project categories and labels.
package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task"
)

// maxBodyBytes limits every request body.
const maxBodyBytes = 1 << 20

// Error codes of the {"error":{"code","message"}} body.
const (
	codeValidationFailed  = "validation_failed"
	codeNotFound          = "not_found"
	codeConflict          = "conflict"
	codeInvalidTransition = "invalid_transition"
	codeUnauthorized      = "unauthorized"
	codePayloadTooLarge   = "payload_too_large"
	codeUnavailable       = "unavailable"
	codeInternal          = "internal_error"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// writeDomainError maps the Task domain errors to HTTP responses. Unexpected
// errors are logged and answered with a generic 500.
func writeDomainError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	var ve *task.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusUnprocessableEntity, codeValidationFailed, ve.Error())
	case errors.Is(err, task.ErrFieldNotEditable):
		writeError(w, http.StatusUnprocessableEntity, codeValidationFailed, err.Error())
	case errors.Is(err, task.ErrNotFound):
		writeError(w, http.StatusNotFound, codeNotFound, "resource not found")
	case errors.Is(err, task.ErrInvalidTransition):
		writeError(w, http.StatusConflict, codeInvalidTransition, err.Error())
	case errors.Is(err, task.ErrConflict):
		writeError(w, http.StatusConflict, codeConflict, err.Error())
	default:
		log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
	}
}

// readBody reads at most maxBodyBytes, answering 413 when the body is larger.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge,
			fmt.Sprintf("body must have at most %d bytes", maxBodyBytes))
		return nil, false
	case err != nil:
		writeError(w, http.StatusBadRequest, codeValidationFailed, "could not read the request body")
		return nil, false
	}
	return body, true
}

// decodeJSON decodes a single JSON object into dst, rejecting unknown fields.
// It answers the error itself and reports whether decoding succeeded.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, ok := readBody(w, r)
	if !ok {
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, codeValidationFailed, "invalid JSON body: "+err.Error())
		return false
	}
	if dec.More() {
		writeError(w, http.StatusBadRequest, codeValidationFailed, "invalid JSON body: unexpected data after the object")
		return false
	}
	return true
}

// pathID parses the {id} path value; an id that is not a UUID cannot exist,
// so it is answered with 404.
func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "resource not found")
		return uuid.UUID{}, false
	}
	return id, true
}

// Optional is a JSON field that tells "absent" from "null": Set is true when
// the field was present, and Value is nil when it was null.
type Optional[T any] struct {
	Set   bool
	Value *T
}

func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}
