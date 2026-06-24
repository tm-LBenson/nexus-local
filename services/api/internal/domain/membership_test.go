package domain

import (
	"errors"
	"testing"
)

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		role       Role
		permission Permission
		want       bool
	}{
		{RoleOwner, PermissionManageTenant, true},
		{RoleAdmin, PermissionManageTenant, true},
		{RoleMember, PermissionManageTenant, false},
		{RoleMember, PermissionUseAI, true},
		{RoleViewer, PermissionUploadDocuments, false},
		{RoleViewer, PermissionReadDocuments, true},
	}

	for _, tc := range cases {
		got := tc.role.Allows(tc.permission)
		if got != tc.want {
			t.Fatalf("%s allows %s = %v, want %v", tc.role, tc.permission, got, tc.want)
		}
	}
}

func TestNewMembershipRequiresKnownRole(t *testing.T) {
	_, err := NewMembership(MembershipCreate{
		TenantID: TenantID("tenant_1"),
		UserID:   UserID("user_1"),
		Role:     Role("superuser"),
	})
	if !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("err = %v, want ErrInvalidEntity", err)
	}
}
