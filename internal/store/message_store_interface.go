package store

import (
	"context"
	"go-im/internal/models"
	"time"
)

// MessageStoreInterface 抽象消息存储，便于切换 MySQL/TiDB/MongoDB：
// - Append：写入消息（需具备幂等约束）
// - Recall/DeleteConversation：撤回消息/设置删除水位
// - List：按会话游标拉取历史
// - DeleteExpired：清理到期的定时自毁
// - RecallBySeq/GetBySeq：按序处理（阅后即焚依赖）
type MessageStoreInterface interface {
	// Append 写入消息；要求底层实现对 (conv_id, client_msg_id) 提供唯一约束以实现幂等。
	Append(ctx context.Context, m *models.Message) error
	// Recall 将消息标记为撤回（不物理删除）。
	Recall(ctx context.Context, convID, serverMsgID string) error
	// DeleteConversation 记录会话删除水位（owner 视角）。
	DeleteConversation(ctx context.Context, ownerID, convID string) error
	// List 拉取历史（按 seq 严格递增，返回量受 limit 控制）。
	List(ctx context.Context, convID string, fromSeq int64, limit int) ([]*models.Message, error)
	// DeleteExpired 清理到期自毁消息（可由后台任务周期调用）。
	DeleteExpired(ctx context.Context, before time.Time) error
	// RecallBySeq 将会话内指定 seq 的消息标记撤回（用于阅后即焚）。
	RecallBySeq(ctx context.Context, convID string, seq int64) error
	// GetBySeq 查询会话内 seq 对应的消息（用于判断 burnAfterRead 等属性）。
	GetBySeq(ctx context.Context, convID string, seq int64) (*models.Message, error)
}
