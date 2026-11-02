package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openpak/ps5-common/oauth"

	"openpak/ps5-friends/internal/social"
)

type env struct {
	t   *testing.T
	h   http.Handler
	sg  *oauth.Signer
	srv *Server
}

func newEnv(t *testing.T) *env {
	sg, err := oauth.NewSigner([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: social.NewMemory()}
	return &env{t, s.Handler(sg), sg, s}
}

func (e *env) do(acct, method, path, body string) *httptest.ResponseRecorder {
	tok, _ := e.sg.Sign(oauth.Claims{AccountID: acct, Expires: time.Now().Add(time.Hour).Unix()})
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

const base = "/api/userProfile/v1/internal/users/"

func (e *env) want(w *httptest.ResponseRecorder, code int, sub string) {
	e.t.Helper()
	if w.Code != code || !strings.Contains(w.Body.String(), sub) {
		e.t.Fatalf("got %d %s, want %d containing %q", w.Code, w.Body.String(), code, sub)
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, httptest.NewRequest("GET", base+"me/friends", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestFriendFlow(t *testing.T) {
	e := newEnv(t)
	e.want(e.do("1", "PUT", base+"me/friends/2", ""), 204, "")
	e.want(e.do("2", "GET", base+"me/friends/receivedRequests", ""), 200, `"friends":["1"]`)
	e.want(e.do("2", "GET", base+"me/friends/requests", ""), 200, `"friends":["1"]`)
	e.want(e.do("1", "GET", base+"me/friends/2/summary", ""), 200, `"requested"`)
	e.want(e.do("1", "GET", base+"me/friends", ""), 200, `"friends":[]`)
	// accept
	e.want(e.do("2", "PUT", base+"me/friends/1", ""), 204, "")
	e.want(e.do("1", "GET", base+"me/friends", ""), 200, `"friends":["2"],"totalItemCount":1`)
	e.want(e.do("2", "GET", base+"1/friends", ""), 200, `"friends":["2"]`)
	e.want(e.do("3", "GET", base+"1/friends", ""), 403, "Not permitted by access control")
	e.want(e.do("2", "GET", base+"me/friends/receivedRequests", ""), 200, `"totalItemCount":0`)
	// someone else's graph is not editable
	e.want(e.do("3", "PUT", base+"1/friends/3", ""), 403, "")
	e.want(e.do("1", "PUT", base+"me/friends/1", ""), 400, "")
	e.want(e.do("1", "DELETE", base+"me/friends/2", ""), 204, "")
	e.want(e.do("1", "DELETE", base+"me/friends/2", ""), 404, "")
	e.want(e.do("2", "GET", base+"me/friends", ""), 200, `"friends":[]`)
}

func TestPaging(t *testing.T) {
	e := newEnv(t)
	for _, o := range []string{"10", "11", "12"} {
		e.do("1", "PUT", base+"me/friends/"+o, "")
		e.do(o, "PUT", base+"me/friends/1", "")
	}
	e.want(e.do("1", "GET", base+"me/friends?limit=2", ""), 200, `"nextOffset":2`)
	e.want(e.do("1", "GET", base+"me/friends?limit=2&offset=2", ""), 200, `"previousOffset":0`)
}

func TestBlock(t *testing.T) {
	e := newEnv(t)
	e.do("1", "PUT", base+"me/friends/2", "")
	e.do("2", "PUT", base+"me/friends/1", "")
	e.want(e.do("1", "PUT", base+"me/blocks/2", ""), 204, "")
	e.want(e.do("1", "GET", base+"me/blocks", ""), 200, `"blockList":["2"]`)
	e.want(e.do("1", "GET", base+"me/friends", ""), 200, `"friends":[]`)
	e.want(e.do("2", "PUT", base+"me/friends/1", ""), 403, "")
	e.want(e.do("1", "DELETE", base+"me/blocks/2", ""), 204, "")
	e.want(e.do("1", "DELETE", base+"me/blocks/2", ""), 404, "")
	e.want(e.do("2", "PUT", base+"me/friends/1", ""), 204, "")
}

func TestPresence(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	e.srv.Now = func() time.Time { return now }
	e.do("1", "PUT", base+"me/friends/2", "")
	e.do("2", "PUT", base+"me/friends/1", "")
	// inferred: 2 used its token just now
	e.want(e.do("1", "GET", base+"2/basicPresences?type=primary", ""), 200, `"availability":"availableToPlay"`)
	// explicit
	e.want(e.do("2", "PUT", base+"me/basicPresences", `{"availability":"availableToPlay","platform":"PS5","gameTitleInfoList":[{"npTitleId":"PPSA00001_00","titleName":"Demo","format":"PS5"}]}`), 204, "")
	e.want(e.do("1", "GET", base+"2/basicPresences", ""), 200, `"npTitleId":"PPSA00001_00"`)
	e.want(e.do("3", "GET", base+"2/basicPresences", ""), 403, "")
	// token idle past the TTL: unavailable
	now = now.Add(PresenceTTL + time.Minute)
	e.srv.Now = func() time.Time { return now }
	w := e.do("1", "GET", base+"2/basicPresences", "")
	e.want(w, 200, `"availability":"unavailable"`)
	e.want(w, 200, `"onlineStatus":"offline"`)
}
