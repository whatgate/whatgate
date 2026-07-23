package trust

import "testing"

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestCreateGroupAddsFounderAsMember(t *testing.T) {
	g := NewGraph()
	g.CreateGroup("g1", "alice")

	if !contains(g.GroupsOf("alice"), "g1") {
		t.Fatalf("founder alice should be a member of g1; got %v", g.GroupsOf("alice"))
	}
}

func TestAddMemberJoinsGroup(t *testing.T) {
	g := NewGraph()
	g.CreateGroup("g1", "alice")
	g.AddMember("g1", "bob")

	if !contains(g.GroupsOf("bob"), "g1") {
		t.Fatalf("bob should be a member of g1; got %v", g.GroupsOf("bob"))
	}
}

func TestIsMember(t *testing.T) {
	g := NewGraph()
	g.CreateGroup("g1", "alice")

	if !g.IsMember("g1", "alice") {
		t.Error("alice should be a member of g1")
	}
	if g.IsMember("g1", "bob") {
		t.Error("bob is not a member of g1")
	}
	if g.IsMember("nope", "alice") {
		t.Error("no such group")
	}
}

func TestPeerCanBelongToMultipleGroups(t *testing.T) {
	g := NewGraph()
	g.CreateGroup("g1", "alice")
	g.CreateGroup("g2", "carol")
	g.AddMember("g2", "alice")

	groups := g.GroupsOf("alice")
	if !contains(groups, "g1") || !contains(groups, "g2") {
		t.Fatalf("alice should be in g1 and g2; got %v", groups)
	}
}
