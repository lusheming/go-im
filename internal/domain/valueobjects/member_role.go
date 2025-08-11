package valueobjects

import "errors"

// MemberRole 成员角色值对象
type MemberRole string

const (
	MemberRoleOwner  MemberRole = "owner"  // 群主
	MemberRoleAdmin  MemberRole = "admin"  // 管理员
	MemberRoleMember MemberRole = "member" // 普通成员
)

// NewMemberRole 创建成员角色值对象
func NewMemberRole(value string) (MemberRole, error) {
	role := MemberRole(value)
	if !role.IsValid() {
		return "", errors.New("无效的成员角色")
	}
	return role, nil
}

// IsValid 验证角色是否有效
func (mr MemberRole) IsValid() bool {
	switch mr {
	case MemberRoleOwner, MemberRoleAdmin, MemberRoleMember:
		return true
	default:
		return false
	}
}

// String 返回字符串表示
func (mr MemberRole) String() string {
	return string(mr)
}

// IsOwner 是否为群主
func (mr MemberRole) IsOwner() bool {
	return mr == MemberRoleOwner
}

// IsAdmin 是否为管理员
func (mr MemberRole) IsAdmin() bool {
	return mr == MemberRoleAdmin
}

// IsMember 是否为普通成员
func (mr MemberRole) IsMember() bool {
	return mr == MemberRoleMember
}

// CanManageGroup 是否有群组管理权限
func (mr MemberRole) CanManageGroup() bool {
	return mr == MemberRoleOwner || mr == MemberRoleAdmin
}

// CanManageMembers 是否有成员管理权限
func (mr MemberRole) CanManageMembers() bool {
	return mr == MemberRoleOwner || mr == MemberRoleAdmin
}
