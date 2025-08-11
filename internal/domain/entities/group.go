package entities

import (
	"errors"
	"time"

	"go-im/internal/domain/valueobjects"
)

// Group 群组领域实体
type Group struct {
	id        string
	name      string
	ownerID   string
	muted     bool
	createdAt time.Time
	updatedAt time.Time
}

// NewGroup 创建新群组实体
func NewGroup(id, name, ownerID string) (*Group, error) {
	if id == "" {
		return nil, errors.New("群组ID不能为空")
	}
	if name == "" {
		return nil, errors.New("群组名称不能为空")
	}
	if ownerID == "" {
		return nil, errors.New("群主ID不能为空")
	}

	now := time.Now()
	return &Group{
		id:        id,
		name:      name,
		ownerID:   ownerID,
		muted:     false,
		createdAt: now,
		updatedAt: now,
	}, nil
}

// ID 获取群组ID
func (g *Group) ID() string {
	return g.id
}

// Name 获取群组名称
func (g *Group) Name() string {
	return g.name
}

// OwnerID 获取群主ID
func (g *Group) OwnerID() string {
	return g.ownerID
}

// IsMuted 是否全员禁言
func (g *Group) IsMuted() bool {
	return g.muted
}

// CreatedAt 获取创建时间
func (g *Group) CreatedAt() time.Time {
	return g.createdAt
}

// UpdatedAt 获取更新时间
func (g *Group) UpdatedAt() time.Time {
	return g.updatedAt
}

// UpdateName 更新群组名称
func (g *Group) UpdateName(name string) error {
	if name == "" {
		return errors.New("群组名称不能为空")
	}
	g.name = name
	g.updatedAt = time.Now()
	return nil
}

// SetMute 设置全员禁言
func (g *Group) SetMute(muted bool) {
	g.muted = muted
	g.updatedAt = time.Now()
}

// TransferOwnership 转让群主
func (g *Group) TransferOwnership(newOwnerID string) error {
	if newOwnerID == "" {
		return errors.New("新群主ID不能为空")
	}
	if newOwnerID == g.ownerID {
		return errors.New("不能转让给当前群主")
	}
	g.ownerID = newOwnerID
	g.updatedAt = time.Now()
	return nil
}

// ToDTO 转换为数据传输对象
func (g *Group) ToDTO() GroupDTO {
	return GroupDTO{
		ID:        g.id,
		Name:      g.name,
		OwnerID:   g.ownerID,
		Muted:     g.muted,
		CreatedAt: g.createdAt,
		UpdatedAt: g.updatedAt,
	}
}

// GroupDTO 群组数据传输对象
type GroupDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	OwnerID     string    `json:"ownerId"`
	Muted       bool      `json:"muted,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	MemberCount int       `json:"memberCount,omitempty"` // 成员数量（管理后台使用）
}

// FromGroupDTO 从DTO创建群组实体
func FromGroupDTO(dto GroupDTO) *Group {
	return &Group{
		id:        dto.ID,
		name:      dto.Name,
		ownerID:   dto.OwnerID,
		muted:     dto.Muted,
		createdAt: dto.CreatedAt,
		updatedAt: dto.UpdatedAt,
	}
}

// GroupMember 群组成员实体
type GroupMember struct {
	groupID    string
	userID     string
	role       valueobjects.MemberRole
	remark     string
	mutedUntil *time.Time
	createdAt  time.Time
	updatedAt  time.Time
}

// NewGroupMember 创建新群组成员
func NewGroupMember(groupID, userID string, role valueobjects.MemberRole, remark string) (*GroupMember, error) {
	if groupID == "" {
		return nil, errors.New("群组ID不能为空")
	}
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	now := time.Now()
	return &GroupMember{
		groupID:   groupID,
		userID:    userID,
		role:      role,
		remark:    remark,
		createdAt: now,
		updatedAt: now,
	}, nil
}

// GroupID 获取群组ID
func (gm *GroupMember) GroupID() string {
	return gm.groupID
}

// UserID 获取用户ID
func (gm *GroupMember) UserID() string {
	return gm.userID
}

// Role 获取角色
func (gm *GroupMember) Role() valueobjects.MemberRole {
	return gm.role
}

// Remark 获取备注
func (gm *GroupMember) Remark() string {
	return gm.remark
}

// MutedUntil 获取禁言截止时间
func (gm *GroupMember) MutedUntil() *time.Time {
	return gm.mutedUntil
}

// CreatedAt 获取创建时间
func (gm *GroupMember) CreatedAt() time.Time {
	return gm.createdAt
}

// UpdatedAt 获取更新时间
func (gm *GroupMember) UpdatedAt() time.Time {
	return gm.updatedAt
}

// UpdateRole 更新角色
func (gm *GroupMember) UpdateRole(role valueobjects.MemberRole) {
	gm.role = role
	gm.updatedAt = time.Now()
}

// UpdateRemark 更新备注
func (gm *GroupMember) UpdateRemark(remark string) {
	gm.remark = remark
	gm.updatedAt = time.Now()
}

// SetMuteUntil 设置禁言截止时间
func (gm *GroupMember) SetMuteUntil(until *time.Time) {
	gm.mutedUntil = until
	gm.updatedAt = time.Now()
}

// IsMuted 检查是否被禁言
func (gm *GroupMember) IsMuted(now time.Time) bool {
	return gm.mutedUntil != nil && gm.mutedUntil.After(now)
}

// ToDTO 转换为数据传输对象
func (gm *GroupMember) ToDTO() GroupMemberDTO {
	return GroupMemberDTO{
		GroupID:    gm.groupID,
		UserID:     gm.userID,
		Role:       gm.role.String(),
		Remark:     gm.remark,
		MutedUntil: gm.mutedUntil,
		CreatedAt:  gm.createdAt,
		UpdatedAt:  gm.updatedAt,
	}
}

// GroupMemberDTO 群组成员数据传输对象
type GroupMemberDTO struct {
	GroupID    string     `json:"groupId"`
	UserID     string     `json:"userId"`
	Role       string     `json:"role"`
	Remark     string     `json:"remark"`
	MutedUntil *time.Time `json:"mutedUntil,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// FromGroupMemberDTO 从DTO创建群组成员实体
func FromGroupMemberDTO(dto GroupMemberDTO) (*GroupMember, error) {
	role, err := valueobjects.NewMemberRole(dto.Role)
	if err != nil {
		return nil, err
	}

	return &GroupMember{
		groupID:    dto.GroupID,
		userID:     dto.UserID,
		role:       role,
		remark:     dto.Remark,
		mutedUntil: dto.MutedUntil,
		createdAt:  dto.CreatedAt,
		updatedAt:  dto.UpdatedAt,
	}, nil
}
