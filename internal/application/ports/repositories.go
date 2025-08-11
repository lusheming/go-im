package ports

import (
	"context"
	"time"

	"go-im/internal/domain/entities"
)

// UserRepository 用户仓储端口
// 定义用户数据访问的抽象接口，由基础设施层实现
type UserRepository interface {
	// Save 保存用户
	Save(ctx context.Context, user *entities.User) error
	// GetByID 根据ID获取用户
	GetByID(ctx context.Context, id string) (*entities.User, error)
	// GetByUsername 根据用户名获取用户
	GetByUsername(ctx context.Context, username string) (*entities.User, error)
	// Update 更新用户信息
	Update(ctx context.Context, user *entities.User) error
	// Delete 删除用户
	Delete(ctx context.Context, id string) error
	// List 分页获取用户列表
	List(ctx context.Context, offset, limit int) ([]*entities.User, error)
	// Count 获取用户总数
	Count(ctx context.Context) (int, error)
}

// MessageRepository 消息仓储端口
type MessageRepository interface {
	// Save 保存消息
	Save(ctx context.Context, message *entities.Message) error
	// GetByID 根据服务器消息ID获取消息
	GetByID(ctx context.Context, serverMsgID string) (*entities.Message, error)
	// GetBySeq 根据会话ID和序号获取消息
	GetBySeq(ctx context.Context, convID string, seq int64) (*entities.Message, error)
	// List 分页获取会话消息列表
	List(ctx context.Context, convID string, fromSeq int64, limit int) ([]*entities.Message, error)
	// Recall 撤回消息
	Recall(ctx context.Context, serverMsgID string) error
	// RecallBySeq 根据序号撤回消息
	RecallBySeq(ctx context.Context, convID string, seq int64) error
	// DeleteExpired 删除过期消息
	DeleteExpired(ctx context.Context, before time.Time) error
	// DeleteConversation 删除会话（设置删除水位）
	DeleteConversation(ctx context.Context, ownerID, convID string) error
	// Count 统计消息数量
	Count(ctx context.Context, convID string) (int, error)
}

// GroupRepository 群组仓储端口
type GroupRepository interface {
	// Save 保存群组
	Save(ctx context.Context, group *entities.Group) error
	// GetByID 根据ID获取群组
	GetByID(ctx context.Context, id string) (*entities.Group, error)
	// Update 更新群组信息
	Update(ctx context.Context, group *entities.Group) error
	// Delete 删除群组
	Delete(ctx context.Context, id string) error
	// List 分页获取群组列表
	List(ctx context.Context, offset, limit int) ([]*entities.Group, error)
	// Count 获取群组总数
	Count(ctx context.Context) (int, error)
}

// GroupMemberRepository 群组成员仓储端口
type GroupMemberRepository interface {
	// Save 保存群组成员
	Save(ctx context.Context, member *entities.GroupMember) error
	// GetByGroupAndUser 获取群组成员
	GetByGroupAndUser(ctx context.Context, groupID, userID string) (*entities.GroupMember, error)
	// Update 更新群组成员信息
	Update(ctx context.Context, member *entities.GroupMember) error
	// Delete 删除群组成员
	Delete(ctx context.Context, groupID, userID string) error
	// ListByGroup 获取群组所有成员
	ListByGroup(ctx context.Context, groupID string) ([]*entities.GroupMember, error)
	// ListByUser 获取用户加入的所有群组
	ListByUser(ctx context.Context, userID string) ([]*entities.GroupMember, error)
	// IsMember 检查是否为群组成员
	IsMember(ctx context.Context, groupID, userID string) (bool, error)
	// Count 统计群组成员数量
	Count(ctx context.Context, groupID string) (int, error)
}

// ConversationRepository 会话仓储端口
type ConversationRepository interface {
	// Save 保存会话
	Save(ctx context.Context, conv *ConversationEntity) error
	// GetByID 根据ID获取会话
	GetByID(ctx context.Context, id string) (*ConversationEntity, error)
	// Update 更新会话信息
	Update(ctx context.Context, conv *ConversationEntity) error
	// Delete 删除会话
	Delete(ctx context.Context, id string) error
	// ListByOwner 获取用户的会话列表
	ListByOwner(ctx context.Context, ownerID string, offset, limit int) ([]*ConversationEntity, error)
}

// ConversationEntity 会话实体（简化版，可后续扩展）
type ConversationEntity struct {
	ID         string
	Type       string
	OwnerID    string
	PeerID     string
	GroupID    string
	LastMsgSeq int64
	UpdatedAt  time.Time
}

// FriendRepository 好友仓储端口
type FriendRepository interface {
	// Save 保存好友关系
	Save(ctx context.Context, friend *FriendEntity) error
	// GetByUsers 获取好友关系
	GetByUsers(ctx context.Context, userID, friendID string) (*FriendEntity, error)
	// Delete 删除好友关系
	Delete(ctx context.Context, userID, friendID string) error
	// ListByUser 获取用户的好友列表
	ListByUser(ctx context.Context, userID string) ([]*FriendEntity, error)
	// IsFriend 检查是否为好友关系
	IsFriend(ctx context.Context, userID, friendID string) (bool, error)
}

// FriendEntity 好友关系实体（简化版）
type FriendEntity struct {
	UserID    string
	FriendID  string
	Remark    string
	CreatedAt time.Time
	UpdatedAt time.Time
}
