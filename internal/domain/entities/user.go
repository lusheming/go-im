package entities

import (
	"errors"
	"time"
)

// User 用户领域实体
// 包含用户的核心业务属性和行为，不依赖任何外部技术
type User struct {
	id        string
	username  string
	password  string
	nickname  string
	avatarURL string
	createdAt time.Time
	updatedAt time.Time
}

// NewUser 创建新用户实体
func NewUser(id, username, password, nickname string) (*User, error) {
	if id == "" {
		return nil, errors.New("用户ID不能为空")
	}
	if username == "" {
		return nil, errors.New("用户名不能为空")
	}
	if password == "" {
		return nil, errors.New("密码不能为空")
	}

	now := time.Now()
	return &User{
		id:        id,
		username:  username,
		password:  password,
		nickname:  nickname,
		createdAt: now,
		updatedAt: now,
	}, nil
}

// ID 获取用户ID
func (u *User) ID() string {
	return u.id
}

// Username 获取用户名
func (u *User) Username() string {
	return u.username
}

// Password 获取密码
func (u *User) Password() string {
	return u.password
}

// Nickname 获取昵称
func (u *User) Nickname() string {
	return u.nickname
}

// AvatarURL 获取头像URL
func (u *User) AvatarURL() string {
	return u.avatarURL
}

// CreatedAt 获取创建时间
func (u *User) CreatedAt() time.Time {
	return u.createdAt
}

// UpdatedAt 获取更新时间
func (u *User) UpdatedAt() time.Time {
	return u.updatedAt
}

// UpdateNickname 更新昵称
func (u *User) UpdateNickname(nickname string) {
	u.nickname = nickname
	u.updatedAt = time.Now()
}

// UpdateAvatarURL 更新头像URL
func (u *User) UpdateAvatarURL(avatarURL string) {
	u.avatarURL = avatarURL
	u.updatedAt = time.Now()
}

// UpdateProfile 更新用户资料
func (u *User) UpdateProfile(nickname, avatarURL string) {
	u.nickname = nickname
	u.avatarURL = avatarURL
	u.updatedAt = time.Now()
}

// IsValidPassword 验证密码（业务逻辑）
func (u *User) IsValidPassword(password string) bool {
	// 这里应该使用加密验证，简化为直接比较
	return u.password == password
}

// ChangePassword 修改密码
func (u *User) ChangePassword(newPassword string) error {
	if newPassword == "" {
		return errors.New("新密码不能为空")
	}
	u.password = newPassword
	u.updatedAt = time.Now()
	return nil
}

// ToDTO 转换为数据传输对象（用于跨层传输）
func (u *User) ToDTO() UserDTO {
	return UserDTO{
		ID:        u.id,
		Username:  u.username,
		Nickname:  u.nickname,
		AvatarURL: u.avatarURL,
		CreatedAt: u.createdAt,
		UpdatedAt: u.updatedAt,
	}
}

// UserDTO 用户数据传输对象
// 用于在不同层之间传输用户数据，不包含密码等敏感信息
type UserDTO struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	AvatarURL string    `json:"avatarUrl"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Online    bool      `json:"online,omitempty"` // 在线状态（管理后台使用）
}

// FromDTO 从DTO创建用户实体（用于从存储层恢复）
func FromUserDTO(dto UserDTO, password string) *User {
	return &User{
		id:        dto.ID,
		username:  dto.Username,
		password:  password,
		nickname:  dto.Nickname,
		avatarURL: dto.AvatarURL,
		createdAt: dto.CreatedAt,
		updatedAt: dto.UpdatedAt,
	}
}
