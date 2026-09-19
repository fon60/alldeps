package domain

import "testing"

func TestOneManagerPerDestination(t *testing.T) {
	ok := &Plan{Ops: []Op{
		{Destination: "/node/v16", Manager: "npm", Name: "alpha", Mark: MarkInstall},
		{Destination: "/node/v18", Manager: "yarn", Name: "beta", Mark: MarkInstall},
	}}
	if !ok.Valid() {
		t.Fatalf("npm@v16 + yarn@v18 must be valid (distinct destinations), invalid = %v", ok.InvalidDestinations())
	}

	bad := &Plan{Ops: []Op{
		{Destination: "/node/v18", Manager: "npm", Name: "alpha", Mark: MarkInstall},
		{Destination: "/node/v18", Manager: "yarn", Name: "beta", Mark: MarkRemove},
	}}
	if bad.Valid() {
		t.Fatal("npm@v18 + yarn@v18 must be rejected (two managers, one destination)")
	}
	if got := bad.InvalidDestinations(); len(got) != 1 || got[0] != "/node/v18" {
		t.Fatalf("invalid destinations = %v, want [/node/v18]", got)
	}

	mixed := &Plan{Ops: []Op{
		{Destination: "/node/v16", Manager: "npm", Name: "a", Mark: MarkInstall},
		{Destination: "/node/v16", Manager: "npm", Name: "b", Mark: MarkRemove},
		{Destination: "/system", Manager: "composer", Name: "c", Mark: MarkInstall},
	}}
	if !mixed.Valid() {
		t.Fatalf("same manager on one destination plus another manager elsewhere must be valid, invalid = %v", mixed.InvalidDestinations())
	}
}
