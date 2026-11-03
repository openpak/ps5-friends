// Package server is the HTTP face of ps5-friends: the friends, friend
// request, block and basic presence parts of the NP web API 2 userProfile
// service on m.np.playstation.com.
package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/openpak/ps5-common/httpx"
	"github.com/openpak/ps5-common/ids"
	"github.com/openpak/ps5-common/oauth"

	"openpak/ps5-friends/internal/social"
)

// Prefix is the one path prefix this service owns (ps5-profile owns the
// /profiles leaves of the same tree; see docs/deploy.md).
const Prefix = "/api/userProfile/v1/internal/users/{id}"

// PresenceTTL is how long after its last authenticated request an account
// counts as online.
const PresenceTTL = 5 * time.Minute

// PSN-shaped error codes. The numeric values are placeholders (unverified).
const (
	codeBadRequest = 2240001
	codeForbidden  = 2240526
	codeNotFound   = 2240002
)

// Server serves the friends API.
type Server struct {
	Store social.Store
	Now   func() time.Time
}

// Handler returns the full mux, authenticated with v.
func (s *Server) Handler(v oauth.Verifier) http.Handler {
	if s.Now == nil {
		s.Now = time.Now
	}
	mux := httpx.Mux()
	auth := oauth.Require(v)
	h := func(f http.HandlerFunc) http.Handler { return auth(s.touch(f)) }
	mux.Handle("GET "+Prefix+"/friends", h(s.listFriends))
	mux.Handle("GET "+Prefix+"/friends/{other}", h(s.friendLeaf))
	mux.Handle("PUT "+Prefix+"/friends/{other}", h(s.requestFriend))
	mux.Handle("DELETE "+Prefix+"/friends/{other}", h(s.removeFriend))
	mux.Handle("GET "+Prefix+"/friends/{other}/summary", h(s.summary))
	mux.Handle("GET "+Prefix+"/blocks", h(s.listBlocks))
	mux.Handle("PUT "+Prefix+"/blocks/{other}", h(s.block))
	mux.Handle("DELETE "+Prefix+"/blocks/{other}", h(s.unblock))
	mux.Handle("GET "+Prefix+"/basicPresences", h(s.getPresence))
	mux.Handle("PUT "+Prefix+"/basicPresences", h(s.putPresence))
	return mux
}

// touch records that the caller's token was just used (presence).
func (s *Server) touch(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, ok := oauth.FromContext(r.Context()); ok {
			s.Store.Touch(c.AccountID)
		}
		next(w, r)
	}
}

func bad(w http.ResponseWriter, msg string) { httpx.WriteError(w, 400, codeBadRequest, msg) }
func forbidden(w http.ResponseWriter) {
	httpx.WriteError(w, 403, codeForbidden, "Not permitted by access control")
}

// ids resolves {id} ("me" or an account id) and the caller.
func target(w http.ResponseWriter, r *http.Request, name string) (me, id string, ok bool) {
	c, _ := oauth.FromContext(r.Context())
	me = c.AccountID
	id = r.PathValue(name)
	if id == "me" {
		id = me
	}
	if _, err := ids.ParseAccountID(id); err != nil {
		bad(w, "Invalid account id")
		return "", "", false
	}
	return me, id, true
}

// ownOnly resolves {id} and requires it to be the caller.
func ownOnly(w http.ResponseWriter, r *http.Request) (me string, ok bool) {
	me, id, ok := target(w, r, "id")
	if !ok {
		return "", false
	}
	if id != me {
		forbidden(w)
		return "", false
	}
	return me, true
}

func page(r *http.Request, all []string) (items []string, total int, next, prev *int) {
	limit, offset := 100, 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = min(v, 1000)
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v > 0 {
		offset = v
	}
	total = len(all)
	offset = min(offset, total)
	end := min(offset+limit, total)
	items = all[offset:end]
	if end < total {
		next = &end
	}
	if offset > 0 {
		p := max(offset-limit, 0)
		prev = &p
	}
	return
}

type listBody struct {
	Friends        []string `json:"friends"`
	TotalItemCount int      `json:"totalItemCount"`
	NextOffset     *int     `json:"nextOffset,omitempty"`
	PreviousOffset *int     `json:"previousOffset,omitempty"`
}

func writeList(w http.ResponseWriter, r *http.Request, all []string) {
	items, total, next, prev := page(r, all)
	httpx.WriteJSON(w, 200, listBody{Friends: items, TotalItemCount: total, NextOffset: next, PreviousOffset: prev})
}

func (s *Server) listFriends(w http.ResponseWriter, r *http.Request) {
	me, id, ok := target(w, r, "id")
	if !ok {
		return
	}
	if id != me && (s.Store.Relation(me, id) != social.RelFriend) {
		forbidden(w)
		return
	}
	writeList(w, r, s.Store.Friends(id))
}

// friendLeaf serves GET .../friends/receivedRequests and .../friends/requests
// (incoming) and .../friends/sentRequests (outgoing); anything else is 404.
func (s *Server) friendLeaf(w http.ResponseWriter, r *http.Request) {
	switch r.PathValue("other") {
	case "receivedRequests", "requests":
		me, ok := ownOnly(w, r)
		if !ok {
			return
		}
		writeList(w, r, s.Store.Received(me))
	case "sentRequests":
		me, ok := ownOnly(w, r)
		if !ok {
			return
		}
		writeList(w, r, s.Store.Sent(me))
	default:
		httpx.WriteError(w, 404, codeNotFound, "Resource not found")
	}
}

func (s *Server) requestFriend(w http.ResponseWriter, r *http.Request) {
	me, ok := ownOnly(w, r)
	if !ok {
		return
	}
	_, other, ok := target(w, r, "other")
	if !ok {
		return
	}
	_, err := s.Store.Request(me, other)
	switch {
	case errors.Is(err, social.ErrSelf):
		bad(w, "Cannot send a friend request to yourself")
	case errors.Is(err, social.ErrBlocked):
		forbidden(w)
	case err != nil:
		httpx.WriteError(w, 500, 1, "Internal error")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) removeFriend(w http.ResponseWriter, r *http.Request) {
	me, ok := ownOnly(w, r)
	if !ok {
		return
	}
	_, other, ok := target(w, r, "other")
	if !ok {
		return
	}
	if err := s.Store.Remove(me, other); err != nil {
		notFoundOrInternal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// notFoundOrInternal answers 404 for "no such relationship" and 500 for a storage failure.
func notFoundOrInternal(w http.ResponseWriter, err error) {
	if errors.Is(err, social.ErrNothing) {
		httpx.WriteError(w, 404, codeNotFound, "Resource not found")
		return
	}
	httpx.WriteError(w, 500, 1, "Internal error")
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	me, ok := ownOnly(w, r)
	if !ok {
		return
	}
	_, other, ok := target(w, r, "other")
	if !ok {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"friendRelation": string(s.Store.Relation(me, other)), "accountId": other})
}

func (s *Server) listBlocks(w http.ResponseWriter, r *http.Request) {
	me, ok := ownOnly(w, r)
	if !ok {
		return
	}
	items, total, next, prev := page(r, s.Store.Blocks(me))
	httpx.WriteJSON(w, 200, map[string]any{"blockList": items, "totalItemCount": total, "nextOffset": next, "previousOffset": prev})
}

func (s *Server) block(w http.ResponseWriter, r *http.Request) {
	me, ok := ownOnly(w, r)
	if !ok {
		return
	}
	_, other, ok := target(w, r, "other")
	if !ok {
		return
	}
	if err := s.Store.Block(me, other); err != nil {
		if errors.Is(err, social.ErrSelf) {
			bad(w, "Cannot block yourself")
		} else {
			httpx.WriteError(w, 500, 1, "Internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) unblock(w http.ResponseWriter, r *http.Request) {
	me, ok := ownOnly(w, r)
	if !ok {
		return
	}
	_, other, ok := target(w, r, "other")
	if !ok {
		return
	}
	if err := s.Store.Unblock(me, other); err != nil {
		notFoundOrInternal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type platformInfo struct {
	OnlineStatus   string `json:"onlineStatus"`
	Platform       string `json:"platform"`
	LastOnlineDate string `json:"lastOnlineDate,omitempty"`
}

type basicPresence struct {
	Availability      string             `json:"availability"`
	LastAvailableDate string             `json:"lastAvailableDate,omitempty"`
	PrimaryPlatform   platformInfo       `json:"primaryPlatformInfo"`
	GameTitleInfoList []social.GameTitle `json:"gameTitleInfoList,omitempty"`
}

func (s *Server) presenceOf(id string) basicPresence {
	p, known := s.Store.Presence(id)
	plat := p.Platform
	if plat == "" {
		plat = "PS5"
	}
	out := basicPresence{Availability: "unavailable", PrimaryPlatform: platformInfo{OnlineStatus: "offline", Platform: plat}}
	if !known {
		return out
	}
	seen := p.LastSeen.UTC().Format("2006-01-02T15:04:05.000Z")
	if s.Now().Sub(p.LastSeen) > PresenceTTL {
		out.LastAvailableDate = seen
		out.PrimaryPlatform.LastOnlineDate = seen
		return out
	}
	av := "availableToPlay"
	if p.Set && p.Availability != "" {
		av = p.Availability
	}
	out.Availability = av
	out.PrimaryPlatform.OnlineStatus = "online"
	out.PrimaryPlatform.LastOnlineDate = seen
	out.LastAvailableDate = seen
	if p.Set {
		out.GameTitleInfoList = p.Titles
	}
	return out
}

func (s *Server) getPresence(w http.ResponseWriter, r *http.Request) {
	me, id, ok := target(w, r, "id")
	if !ok {
		return
	}
	if id != me && (s.Store.Relation(me, id) != social.RelFriend || s.Store.IsBlocked(me, id)) {
		forbidden(w)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"basicPresence": s.presenceOf(id)})
}

// putPresence is an OpenPak extension: the console (or a patch) announces its
// own presence. Body: {"availability","platform","gameTitleInfoList"}.
func (s *Server) putPresence(w http.ResponseWriter, r *http.Request) {
	me, ok := ownOnly(w, r)
	if !ok {
		return
	}
	var b struct {
		Availability      string             `json:"availability"`
		Platform          string             `json:"platform"`
		GameTitleInfoList []social.GameTitle `json:"gameTitleInfoList"`
	}
	if err := httpx.ReadJSON(r, &b); err != nil {
		bad(w, "Invalid body")
		return
	}
	if b.Availability != "" && b.Availability != "availableToPlay" && b.Availability != "unavailable" {
		bad(w, "Invalid availability")
		return
	}
	s.Store.SetPresence(me, social.Presence{Availability: b.Availability, Platform: b.Platform, Titles: b.GameTitleInfoList})
	w.WriteHeader(http.StatusNoContent)
}
