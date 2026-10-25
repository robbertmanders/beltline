// Support for the read routes: the path parameter reader, the error-carrying handler convention, and
// the query helpers.

package routes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
)

// HTTPError is an error with an HTTP status and an optional body message
// (Hummingbird's HTTPError). ErrorMiddleware (server) maps it to a response.
type HTTPError struct {
	Status  int
	Message string
}

// Error implements error for HTTPError: the message, or the status's reason phrase.
func (e HTTPError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return ReasonPhrase(e.Status)
}

// ReasonPhrase is HTTPTypes' reasonPhrase for the statuses the server uses; the
// names follow RFC 9110 where net/http's older names differ (413, 422).
func ReasonPhrase(status int) string {
	switch status {
	case 413:
		return "Content Too Large"
	case 422:
		return "Unprocessable Content"
	}
	return http.StatusText(status)
}

// ReadRouteSupportUnavailable is the 503
// "Coordinator unavailable" error.
func ReadRouteSupportUnavailable() HTTPError {
	return HTTPError{Status: http.StatusServiceUnavailable, Message: "Coordinator unavailable"}
}

// ReadRouteSupportID is the captured `id` path
// segment, decoded once so an encoded slash stays part of the id.
func ReadRouteSupportID(r *http.Request) (string, error) {
	return ReadRouteSupportParameter(r, "id")
}

// ReadRouteSupportParameter is a named
// path segment, decoded once. Go's ServeMux already matches a wildcard against the
// escaped segment and returns the decoded value from PathValue, so the segment is
// decoded exactly once here. A segment whose escapes do not decode to UTF-8 is a
// 400 "Bad Request"; a malformed
// escape never reaches a handler (net/http answers 400 for it).
func ReadRouteSupportParameter(r *http.Request, name string) (string, error) {
	value := r.PathValue(name)
	if !utf8.ValidString(value) {
		return "", HTTPError{Status: http.StatusBadRequest}
	}
	return value, nil
}

// rawPathSegment is the escaped (not yet decoded) path segment at index, counting
// the empty segment before the leading slash as 0. Some parameters are read raw
// (`parameters.require`), so a route that does compares this instead of PathValue.
func rawPathSegment(r *http.Request, index int) string {
	segments := strings.Split(r.URL.EscapedPath(), "/")
	if index < 0 || index >= len(segments) {
		return ""
	}
	return segments[index]
}

// ReadRouteSupportJSON is a JSON
// body read up to 4 MiB. A missing or empty body is nil when optional, else a
// 422 "The request body is required."; a body that is not valid JSON for T is a
// 422 naming the type, which the Go type's name carries (runTypeName). The
// name is not the Go type name, which describes a struct as a dictionary.
func ReadRouteSupportJSON[T any](r *http.Request, optional bool) (*T, error) {
	payload, err := readBodyLimited(r, 4<<20)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		if optional {
			return nil, nil
		}
		return nil, ReadRouteSupportFail("The request body is required.")
	}
	var v T
	if err := JSONDecode(payload, &v); err != nil {
		return nil, ReadRouteSupportFail(fmt.Sprintf("The request body is not valid JSON for %s.", runTypeName[T]()))
	}
	return &v, nil
}

// ReadRouteSupportQuery is the first value of
// a query parameter, percent-decoded when it can be.
func ReadRouteSupportQuery(name, query string) *string {
	values := ReadRouteSupportQueryValues(name, query)
	if len(values) == 0 {
		return nil
	}
	return &values[0]
}

// ReadRouteSupportQueryValues every
// value of a query parameter, in order, with `+` read as a space. A key that
// does not decode never matches; a value that does not decode is kept as sent.
func ReadRouteSupportQueryValues(name, query string) []string {
	var out []string
	for _, pair := range strings.Split(query, "&") {
		rawKey, raw, hasValue := strings.Cut(pair, "=")
		key, ok := RemovingPercentEncoding(strings.ReplaceAll(rawKey, "+", " "))
		if !ok || key != name {
			continue
		}
		if !hasValue {
			raw = ""
		}
		decoded, ok := RemovingPercentEncoding(strings.ReplaceAll(raw, "+", " "))
		if ok {
			out = append(out, decoded)
		} else {
			out = append(out, raw)
		}
	}
	return out
}

// HummingbirdQueryParameter (Hummingbird's
// URI.queryParameters), the reader the AFK, models and factory routes use. Keys
// are compared raw (not decoded, `+` is not a space), the first value wins, and a
// pair without `=` has the empty value. The value is percent-decoded, with `+`
// kept; a value with a malformed escape is kept as sent. Bytes that are not UTF-8
// become U+FFFD, as Hummingbird's String(decoding:) does.
func HummingbirdQueryParameter(name, rawQuery string) (string, bool) {
	for _, pair := range strings.Split(rawQuery, "&") {
		if pair == "" {
			continue
		}
		rawKey, raw, _ := strings.Cut(pair, "=")
		if rawKey != name {
			continue
		}
		return hummingbirdPercentDecode(raw), true
	}
	return "", false
}

// hummingbirdPercentDecode is Hummingbird's percentDecode for a query value: the
// decoded text, or the value as sent when an escape is malformed.
func hummingbirdPercentDecode(s string) string {
	decoded, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return string([]rune(decoded))
}

// ReadRouteSupportCommandBody is the read route support command body:
// the raw body of a command, read up to 4 MiB. An empty body is nil when the
// command takes no operands and a 400 when it does. Malformed JSON is always a
// 400, so an unknown route or a wrong method cannot mutate state.
func ReadRouteSupportCommandBody(r *http.Request, required bool) ([]byte, error) {
	payload, err := readBodyLimited(r, 4<<20)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		if required {
			return nil, HTTPError{Status: http.StatusBadRequest, Message: "The request body is required."}
		}
		return nil, nil
	}
	var fragment any
	if err := json.Unmarshal(payload, &fragment); err != nil {
		return nil, HTTPError{Status: http.StatusBadRequest, Message: "The request body is not valid JSON."}
	}
	return payload, nil
}

// ReadRouteSupportFail is a 422 validation failure
// whose one problem is the message.
func ReadRouteSupportFail(message string) error {
	return core.NewValidationFailure(message, []string{message})
}

// readBodyLimited reads the whole request body, failing with a 413 HTTPError when
// it is longer than limit (Hummingbird's collect(upTo:)).
func readBodyLimited(r *http.Request, limit int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, HTTPError{Status: 413}
	}
	return data, nil
}

// RemovingPercentEncoding is Foundation's removingPercentEncoding: the
// percent-decoded text, or false when a sequence is malformed or the bytes are
// not UTF-8. `+` is not a space here.
func RemovingPercentEncoding(s string) (string, bool) {
	decoded, err := url.PathUnescape(s)
	if err != nil || !utf8.ValidString(decoded) {
		return "", false
	}
	return decoded, true
}

// PercentDecodedQueryValues and
// `TerminalProtocol.queryValues(_:in:)`: every percent-decoded value of the named
// query parameter, in order. A bare key counts as an empty value, a key that does
// not decode never matches, and a value that does not decode is empty.
func PercentDecodedQueryValues(name, query string) []string {
	var values []string
	for _, pair := range strings.Split(query, "&") {
		rawKey, rawValue, found := strings.Cut(pair, "=")
		if key, ok := RemovingPercentEncoding(rawKey); !ok || key != name {
			continue
		}
		if !found {
			values = append(values, "")
			continue
		}
		value, ok := RemovingPercentEncoding(rawValue)
		if !ok {
			value = ""
		}
		values = append(values, value)
	}
	return values
}

// RouteFunc is a route handler that returns the error its request failed with,
// as the handlers throw. The returned error is mapped by the server's
// ErrorMiddleware (422 for a Failure, 404 for a MissingResource, and so on), so
// a handler never writes an error body itself.
type RouteFunc func(w http.ResponseWriter, r *http.Request) error

// ErrorRoute wraps h as an http.Handler. A non-nil error is recorded on the
// request for the ErrorMiddleware; without one (no middleware above) the route
// answers 500 itself.
//
// The handler's context is detached from the client's cancellation
// (context.WithoutCancel, values kept). The handlers hand the coordinator no
// request context, so a browse, a command or a runner check keeps going when the
// client disconnects, and records its result (a cancelled browse would otherwise be
// stored as the catalog's failure).
func ErrorRoute(h RouteFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithoutCancel(r.Context()))
		if err := h(w, r); err != nil {
			if !RecordError(r, err) {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}
	})
}

// CoordinatorRoute is ErrorRoute for a handler that calls the coordinator. While
// there is no coordinator (the test routers build without one) it answers 503
// "Coordinator unavailable" before h runs.
func CoordinatorRoute(c *coordinator.Coordinator, h RouteFunc) http.Handler {
	return ErrorRoute(guardCoordinator(c, h))
}

// JSONRoute is CoordinatorRoute for a handler that answers 200 with the JSON of
// the value it returns, or fails with its error.
func JSONRoute(c *coordinator.Coordinator, h func(r *http.Request) (any, error)) http.Handler {
	return CoordinatorRoute(c, func(w http.ResponseWriter, r *http.Request) error {
		value, err := h(r)
		if err != nil {
			return err
		}
		JSONResponse(w, value, http.StatusOK)
		return nil
	})
}

// guardCoordinator is the nil-coordinator check of CoordinatorRoute, for a
// handler that is returned as a RouteFunc rather than registered directly.
func guardCoordinator(c *coordinator.Coordinator, h RouteFunc) RouteFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		if c == nil {
			return ReadRouteSupportUnavailable()
		}
		return h(w, r)
	}
}

// errorSlotKey keys the ErrorSlot in a request's context.
type errorSlotKey struct{}

// ErrorSlot holds the error a route returned for one request.
type ErrorSlot struct {
	err error
}

// Err returns the recorded error, or nil.
func (s *ErrorSlot) Err() error { return s.err }

// WithErrorSlot returns r carrying a new ErrorSlot, for the ErrorMiddleware to
// read after the routes have run.
func WithErrorSlot(r *http.Request) (*http.Request, *ErrorSlot) {
	slot := &ErrorSlot{}
	ctx := context.WithValue(r.Context(), errorSlotKey{}, slot)
	return r.WithContext(ctx), slot
}

// RecordError stores err in the request's ErrorSlot. It reports false when the
// request carries no slot, so no middleware will write the error.
func RecordError(r *http.Request, err error) bool {
	slot, ok := r.Context().Value(errorSlotKey{}).(*ErrorSlot)
	if !ok || err == nil {
		return false
	}
	slot.err = err
	return true
}
