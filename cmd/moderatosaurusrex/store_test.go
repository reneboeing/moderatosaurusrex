package main

import "testing"

func TestDatabaseInviteCode(t *testing.T) {
	if got := databaseInviteCode("public", ""); got != nil {
		t.Fatalf("public event invite code = %#v; want SQL NULL", got)
	}
	if got := databaseInviteCode("private", "REX-cafe"); got != "REX-cafe" {
		t.Fatalf("private event invite code = %#v; want generated code", got)
	}
}
