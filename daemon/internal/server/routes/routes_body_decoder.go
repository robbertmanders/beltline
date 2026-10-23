package routes

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// DefaultBodyLimit is BodyDecoder's default maxBytes (64 KiB).
const DefaultBodyLimit = 64 * 1024

// bodyNotFormat is the localized description of a body whose value does not fit
// its type: an unknown enum case, or a value of the wrong kind.
const bodyNotFormat = "The data couldn't be read because it isn't in the correct format."

// DecodeBody is a strict decode of a
// JSON request body. An empty body, invalid JSON, a missing field or an
// undeclared field is an error (a domain failure, 422) before any command runs.
// The body is read up to maxBytes.
func DecodeBody[T any](r *http.Request, maxBytes int64) (T, error) {
	var zero T
	payload, err := readBodyLimited(r, maxBytes)
	if err != nil {
		return zero, err
	}
	if len(payload) == 0 {
		return zero, core.NewFailure("A JSON body is required.")
	}
	return decodeStrict[T](payload)
}

// DecodeOptionalBody decodes an optional body:
// an empty body is fallback; anything else is decoded strictly as DecodeBody does.
func DecodeOptionalBody[T any](r *http.Request, fallback T, maxBytes int64) (T, error) {
	payload, err := readBodyLimited(r, maxBytes)
	if err != nil {
		var zero T
		return zero, err
	}
	if len(payload) == 0 {
		return fallback, nil
	}
	return decodeStrict[T](payload)
}

// DecodeEmptyObjectOrNoBody decodes an empty object or no body:
// bodyless commands may omit the body or send exactly `{}`. Any other JSON value,
// malformed JSON or undeclared field is a 422 before the command runs.
func DecodeEmptyObjectOrNoBody(r *http.Request, maxBytes int64) error {
	payload, err := readBodyLimited(r, maxBytes)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(payload, &members); err != nil {
		return core.NewFailure("Expected an empty JSON object.")
	}
	if len(members) > 0 {
		return core.NewFailure("Body must be an empty JSON object.")
	}
	return nil
}

// decodeStrict decodes payload into a T, refusing an undeclared key and a missing
// required field, and maps a failure to the 422 Failure the body decoder throws.
func decodeStrict[T any](payload []byte) (T, error) {
	var v T
	if err := decodeRequest(payload, &v, true); err != nil {
		var zero T
		return zero, bodyFailure(err)
	}
	return v, nil
}

// bodyFailure is the Failure a body that does not decode becomes. A domain
// failure a type's decoder raised passes unchanged, an undeclared key is named as
// the Failure "Unknown field", and a string that is not a case of its enum is a
// format failure.
func bodyFailure(err error) error {
	var failure core.Failure
	if errors.As(err, &failure) {
		return failure
	}
	var missing missingFieldError
	if errors.As(err, &missing) {
		return core.NewFailure("Invalid request body. The data couldn't be read because it is missing.")
	}
	var unknown unknownFieldError
	if errors.As(err, &unknown) {
		return core.NewFailure(unknown.Error())
	}
	var notCase notCaseError
	if errors.As(err, &notCase) {
		return core.NewFailure("Invalid request body. " + bodyNotFormat)
	}
	return core.NewFailure("Invalid request body. " + err.Error())
}
