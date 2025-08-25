package auth

import "testing"

func TestRoleToAccountId(t *testing.T) {
	arn := "arn:aws:iam::123456789012:role/Admin"
	got := RoleToAccountId(arn)
	if got != "123456789012" {
		t.Fatalf("expected account id 123456789012, got %s", got)
	}
}
