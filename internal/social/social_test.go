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
