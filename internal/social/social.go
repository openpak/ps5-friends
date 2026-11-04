// Package social is the friend graph, block list and presence store behind
// ps5-friends. Store is the seam: Memory is the only implementation today; a
// later one talks to the OpenPak universal social service (see docs/deploy.md).
package social

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// Relation is how the caller stands to another account.
type Relation string

// The relations Summary reports.
const (
	RelNone       Relation = "no"
	RelFriend     Relation = "friend"
	RelRequested  Relation = "requested"  // the caller sent a request
	RelRequesting Relation = "requesting" // the other account sent one
)

// Errors returned by Store.
var (
	ErrSelf    = errors.New("cannot target yourself")
	ErrBlocked = errors.New("blocked")
	ErrNothing = errors.New("no such relationship")
)

// Outcome of Request.
type Outcome int

// Request outcomes.
const (
	Sent Outcome = iota
	Accepted
	AlreadyFriends
	AlreadySent
)

// GameTitle is one entry of a presence's gameTitleInfoList.
type GameTitle struct {
	NpTitleID      string `json:"npTitleId"`
	TitleName      string `json:"titleName"`
	Format         string `json:"format,omitempty"`
	LaunchPlatform string `json:"launchPlatform,omitempty"`
	NpTitleIconURL string `json:"npTitleIconUrl,omitempty"`
	ConceptIconURL string `json:"conceptIconUrl,omitempty"`
}

// Presence is what an account last announced.
type Presence struct {
	Availability string // availableToPlay or unavailable
	Platform     string // PS5 or ps4
	Titles       []GameTitle
	Set          bool // announced explicitly (PUT) rather than inferred
	LastSeen     time.Time
}

// Store is the storage seam. All ids are PSN account id strings.
type Store interface {
	Friends(id string) []string  // sorted
	Received(id string) []string // incoming requests, sorted
	Sent(id string) []string     // outgoing requests, sorted
	Blocks(id string) []string   // sorted
	Relation(id, other string) Relation
	IsBlocked(a, b string) bool               // either direction
	Request(from, to string) (Outcome, error) // send, or accept an incoming request
	Remove(id, other string) error            // unfriend, cancel or decline
	Block(id, other string) error             // also severs friendship and requests
	Unblock(id, other string) error
	SetPresence(id string, p Presence)
	Touch(id string) // token seen now
	Presence(id string) (Presence, bool)
	// Erase removes every friend, request, block and presence row that names any of ids, on
	// either side, and reports the rows removed. Idempotent.
	Erase(ids []string) (int, error)
}

type pair struct{ a, b string }

// Memory is the in-memory Store. Safe for concurrent use.
type Memory struct {
	mu      sync.Mutex
	friends map[pair]bool // both orders stored
	reqs    map[pair]bool // from -> to
	blocks  map[pair]bool // blocker -> blocked
	pres    map[string]Presence
	now     func() time.Time
}

// NewMemory returns an empty store.
func NewMemory() *Memory {
	return &Memory{friends: map[pair]bool{}, reqs: map[pair]bool{}, blocks: map[pair]bool{}, pres: map[string]Presence{}, now: time.Now}
}

func list(m map[pair]bool, pick func(pair) (string, bool)) []string {
	out := []string{}
	for k := range m {
		if s, ok := pick(k); ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Memory) Friends(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return list(m.friends, func(k pair) (string, bool) { return k.b, k.a == id })
}

func (m *Memory) Received(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return list(m.reqs, func(k pair) (string, bool) { return k.a, k.b == id })
}

func (m *Memory) Sent(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return list(m.reqs, func(k pair) (string, bool) { return k.b, k.a == id })
}

func (m *Memory) Blocks(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return list(m.blocks, func(k pair) (string, bool) { return k.b, k.a == id })
}

func (m *Memory) Relation(id, other string) Relation {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.friends[pair{id, other}]:
		return RelFriend
	case m.reqs[pair{id, other}]:
		return RelRequested
	case m.reqs[pair{other, id}]:
		return RelRequesting
	}
	return RelNone
}

func (m *Memory) IsBlocked(a, b string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blocks[pair{a, b}] || m.blocks[pair{b, a}]
}

func (m *Memory) Request(from, to string) (Outcome, error) {
	if from == to {
		return 0, ErrSelf
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.blocks[pair{from, to}] || m.blocks[pair{to, from}] {
		return 0, ErrBlocked
	}
	if m.friends[pair{from, to}] {
		return AlreadyFriends, nil
	}
	if m.reqs[pair{to, from}] {
		delete(m.reqs, pair{to, from})
		m.friends[pair{from, to}] = true
		m.friends[pair{to, from}] = true
		return Accepted, nil
	}
	if m.reqs[pair{from, to}] {
		return AlreadySent, nil
	}
	m.reqs[pair{from, to}] = true
	return Sent, nil
}

func (m *Memory) sever(a, b string) bool {
	any := m.friends[pair{a, b}] || m.reqs[pair{a, b}] || m.reqs[pair{b, a}]
	delete(m.friends, pair{a, b})
	delete(m.friends, pair{b, a})
	delete(m.reqs, pair{a, b})
	delete(m.reqs, pair{b, a})
	return any
}

func (m *Memory) Remove(id, other string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.sever(id, other) {
		return ErrNothing
	}
	return nil
}

func (m *Memory) Block(id, other string) error {
	if id == other {
		return ErrSelf
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sever(id, other)
	m.blocks[pair{id, other}] = true
	return nil
}

func (m *Memory) Unblock(id, other string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.blocks[pair{id, other}] {
		return ErrNothing
	}
	delete(m.blocks, pair{id, other})
	return nil
}

func (m *Memory) SetPresence(id string, p Presence) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p.Set = true
	p.LastSeen = m.now()
	m.pres[id] = p
}

func (m *Memory) Touch(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pres[id]
	p.LastSeen = m.now()
	m.pres[id] = p
}

func (m *Memory) Presence(id string) (Presence, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pres[id]
	return p, ok
}

// erasePresence drops the transient presence of ids and reports how many entries it held.
func (m *Memory) erasePresence(ids []string) int {
	n := 0
	for _, id := range ids {
		if _, ok := m.pres[id]; ok {
			delete(m.pres, id)
			n++
		}
	}
	return n
}

// Erase implements Store.
func (m *Memory) Erase(ids []string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	n := 0
	for _, g := range []map[pair]bool{m.friends, m.reqs, m.blocks} {
		for k := range g {
			if set[k.a] || set[k.b] {
				delete(g, k)
				n++
			}
		}
	}
	return n + m.erasePresence(ids), nil
}
