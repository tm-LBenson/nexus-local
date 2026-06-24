package domain

import "fmt"

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

type Permission string

const (
	PermissionManageTenant    Permission = "tenant.manage"
	PermissionManageUsers     Permission = "users.manage"
	PermissionUseAI           Permission = "ai.use"
	PermissionUploadDocuments Permission = "documents.upload"
	PermissionReadDocuments   Permission = "documents.read"
)

type Membership struct {
	TenantID TenantID
	UserID   UserID
	Role     Role
}

type MembershipCreate struct {
	TenantID TenantID
	UserID   UserID
	Role     Role
}

func NewMembership(input MembershipCreate) (Membership, error) {
	if emptyID(string(input.TenantID)) || emptyID(string(input.UserID)) || !input.Role.Valid() {
		return Membership{}, fmt.Errorf("membership: %w", ErrInvalidEntity)
	}

	return Membership{
		TenantID: input.TenantID,
		UserID:   input.UserID,
		Role:     input.Role,
	}, nil
}

func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember, RoleViewer:
		return true
	default:
		return false
	}
}

func (r Role) Allows(permission Permission) bool {
	switch r {
	case RoleOwner:
		return true
	case RoleAdmin:
		return permission == PermissionManageTenant ||
			permission == PermissionManageUsers ||
			permission == PermissionUseAI ||
			permission == PermissionUploadDocuments ||
			permission == PermissionReadDocuments
	case RoleMember:
		return permission == PermissionUseAI ||
			permission == PermissionUploadDocuments ||
			permission == PermissionReadDocuments
	case RoleViewer:
		return permission == PermissionReadDocuments
	default:
		return false
	}
}
