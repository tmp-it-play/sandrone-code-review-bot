package access

import "strings"

type Permission string

const (
	PermissionAdmin    Permission = "admin"
	PermissionMaintain Permission = "maintain"
	PermissionWrite    Permission = "write"
	PermissionTriage   Permission = "triage"
	PermissionRead     Permission = "read"
	PermissionNone     Permission = "none"
)

func ParsePermission(value string) Permission {
	switch Permission(strings.ToLower(strings.TrimSpace(value))) {
	case PermissionAdmin:
		return PermissionAdmin
	case PermissionMaintain:
		return PermissionMaintain
	case PermissionWrite:
		return PermissionWrite
	case PermissionTriage:
		return PermissionTriage
	case PermissionRead:
		return PermissionRead
	default:
		return PermissionNone
	}
}

func (p Permission) CanInvoke() bool {
	return p == PermissionAdmin || p == PermissionMaintain || p == PermissionWrite
}
