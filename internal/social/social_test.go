package social

import "testing"

func TestRequestAcceptBlock(t *testing.T) {
	m := NewMemory()
	if o, _ := m.Request("1", "2"); o != Sent {
		t.Fatal(o)
	}
	if o, _ := m.Request("1", "2"); o != AlreadySent {
		t.Fatal(o)
	}
	if o, _ := m.Request("2", "1"); o != Accepted {
		t.Fatal(o)
	}
	if m.Relation("1", "2") != RelFriend {
		t.Fatal("not friends")
	}
	m.Block("1", "2")
	if m.Relation("2", "1") != RelNone || !m.IsBlocked("2", "1") {
		t.Fatal("block did not sever")
	}
}

func TestMemoryEraseRemovesBothSidesOnly(t *testing.T) {
	m := NewMemory()
	m.Request("1", "2")
	m.Request("2", "1") // friends 1<->2
	m.Request("3", "1") // request into 1
	m.Request("1", "4") // request out of 1
	m.Block("1", "5")
	m.Block("6", "1") // someone else blocked 1
	m.Request("7", "8")
	m.Request("8", "7") // unrelated friends
	m.Block("7", "9")
	m.SetPresence("1", Presence{})
	n, err := m.Erase([]string{"1"})
	if err != nil || n != 7 { // 2 friend rows + 2 requests + 2 blocks + presence
		t.Fatalf("n=%d err=%v", n, err)
	}
	for _, id := range []string{"2", "3", "4", "5", "6"} {
		if len(m.Friends(id))+len(m.Received(id))+len(m.Sent(id))+len(m.Blocks(id)) != 0 || m.Relation(id, "1") != RelNone || m.IsBlocked(id, "1") {
			t.Fatalf("%s still references 1", id)
		}
	}
	if m.Relation("7", "8") != RelFriend || len(m.Blocks("7")) != 1 {
		t.Fatal("unrelated accounts touched")
	}
	if n, err := m.Erase([]string{"1"}); err != nil || n != 0 {
		t.Fatalf("second run n=%d err=%v", n, err)
	}
}
