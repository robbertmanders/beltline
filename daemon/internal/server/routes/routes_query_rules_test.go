// Tests of the query rules: the URI query parameters.

package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// TestHummingbirdQueryParameter checks each rule of Hummingbird's queryParameters.
func TestHummingbirdQueryParameter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, query, want string
		ok                bool
	}{
		{"plusStaysLiteral", "q=sonnet+4", "sonnet+4", true},
		{"percentDecodesValue", "q=sonnet%2B4", "sonnet+4", true},
		{"keysAreComparedRaw", "%71=x", "", false},
		{"keysAreNotDecodedPlusIsNotSpace", "q+x=1", "", false},
		{"valueKeepsLaterEquals", "q=a=b", "a=b", true},
		{"firstValueWins", "q=1&q=2", "1", true},
		{"pairWithoutEqualsIsEmpty", "q", "", true},
		{"malformedEscapeIsKeptAsSent", "q=%ZZ", "%ZZ", true},
		{"emptyPairsAreSkipped", "&&q=x", "x", true},
		{"encodedAsciiDecodes", "q=%61", "a", true},
		{"invalidUTF8BecomesReplacement", "q=%FF", "�", true},
		{"absentKey", "a=1", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, ok := HummingbirdQueryParameter("q", c.query)
			if ok != c.ok || got != c.want {
				t.Fatalf("HummingbirdQueryParameter(q, %q) = %q, %v; want %q, %v", c.query, got, ok, c.want, c.ok)
			}
		})
	}
}

// TestPercentDecodedQueryValues checks the auth and terminal query rule: `+` stays
// literal, a bare key is empty, an undecodable key never matches and an
// undecodable value is empty.
func TestPercentDecodedQueryValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, query string
		want        []string
	}{
		{"plusStaysLiteral", "token=a+b", []string{"a+b"}},
		{"percentDecodesValue", "token=a%2Bb", []string{"a+b"}},
		{"allOccurrencesInOrder", "token=1&x=2&token=3", []string{"1", "3"}},
		{"bareKeyIsEmpty", "token&x=1", []string{""}},
		{"undecodableKeyNeverMatches", "%ZZ=1&token=2", []string{"2"}},
		{"undecodableValueIsEmpty", "token=%ZZ", []string{""}},
		{"emptyQueryHasNoValues", "", nil},
		{"absentKey", "x=1", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := PercentDecodedQueryValues("token", c.query)
			if len(got) != len(c.want) {
				t.Fatalf("PercentDecodedQueryValues(token, %q) = %q; want %q", c.query, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("PercentDecodedQueryValues(token, %q) = %q; want %q", c.query, got, c.want)
				}
			}
		})
	}
}

// TestErrorRouteDetachesCoordinatorWorkFromTheClient checks that a route's context
// is not cancelled when the client has gone (R2 #4, R3 #1–2), and keeps the
// request's error slot for the ErrorMiddleware.
func TestErrorRouteDetachesCoordinatorWorkFromTheClient(t *testing.T) {
	t.Parallel()
	var seen error
	var recorded bool
	h := ErrorRoute(func(w http.ResponseWriter, r *http.Request) error {
		seen = r.Context().Err()
		recorded = RecordError(r, core.NewFailure("kept"))
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, slot := WithErrorSlot(httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seen != nil {
		t.Fatalf("handler context err = %v, want nil after the client went away", seen)
	}
	if !recorded || slot.Err() == nil {
		t.Fatal("the error slot was lost by the detached context")
	}
}
