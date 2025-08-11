package store

import (
	"context"
	"database/sql"
	"time"

	"go-im/internal/models"
)

// MessageStore 基于 SQL 的消息存储实现（MySQL/TiDB 兼容）。
// 约束：
// - messages 表需具备 (conv_id, client_msg_id) 唯一键保障幂等
// - idx_conv_seq 支撑按会话顺序拉取
// - 扩展字段：expire_at、burn_after_read 支持定时自毁/阅后即焚
type MessageStore struct{ DB *sql.DB }

func NewMessageStore(db *sql.DB) *MessageStore { return &MessageStore{DB: db} }

// Append 插入消息；使用 INSERT IGNORE 实现幂等写入。
func (s *MessageStore) Append(ctx context.Context, m *models.Message) error {
	_, err := s.DB.ExecContext(ctx, `INSERT IGNORE INTO messages(server_msg_id, client_msg_id, conv_id, conv_type, from_user_id, to_user_id, group_id, seq, timestamp, type, payload, recalled, expire_at, burn_after_read) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, m.ServerMsgID, m.ClientMsgID, m.ConvID, m.ConvType, m.FromUserID, m.ToUserID, m.GroupID, m.Seq, m.Timestamp, m.Type, m.Payload, m.Recalled, m.ExpireAt, m.BurnAfterRead)
	return err
}

// Recall 标记消息撤回（不删除物理记录）。
func (s *MessageStore) Recall(ctx context.Context, convID, serverMsgID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE messages SET recalled=1 WHERE conv_id=? AND server_msg_id=?`, convID, serverMsgID)
	return err
}

// DeleteConversation 设置用户对会话的删除水位（用于按时间/水位过滤历史视图）。
func (s *MessageStore) DeleteConversation(ctx context.Context, ownerID, convID string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO conv_deletes(owner_id, conv_id, deleted_at) VALUES(?,?,?) ON DUPLICATE KEY UPDATE deleted_at=VALUES(deleted_at)`, ownerID, convID, time.Now())
	return err
}

// List 按会话增量拉取历史：过滤已撤回与已过期消息。
func (s *MessageStore) List(ctx context.Context, convID string, fromSeq int64, limit int) ([]*models.Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT server_msg_id, client_msg_id, conv_id, conv_type, from_user_id, to_user_id, group_id, seq, timestamp, type, payload, recalled, expire_at, burn_after_read FROM messages WHERE conv_id=? AND seq>? AND recalled=0 AND (expire_at IS NULL OR expire_at>NOW()) ORDER BY seq ASC LIMIT ?`, convID, fromSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []*models.Message
	for rows.Next() {
		m := &models.Message{}
		var nt sql.NullTime
		if err := rows.Scan(&m.ServerMsgID, &m.ClientMsgID, &m.ConvID, &m.ConvType, &m.FromUserID, &m.ToUserID, &m.GroupID, &m.Seq, &m.Timestamp, &m.Type, &m.Payload, &m.Recalled, &nt, &m.BurnAfterRead); err != nil {
			return nil, err
		}
		if nt.Valid {
			t := nt.Time
			m.ExpireAt = &t
		}
		res = append(res, m)
	}
	return res, nil
}

// DeleteExpired 物理删除已到期的自毁消息（SQL 侧的简单清理策略）。
func (s *MessageStore) DeleteExpired(ctx context.Context, before time.Time) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM messages WHERE expire_at IS NOT NULL AND expire_at<=?`, before)
	return err
}

// RecallBySeq 按 seq 撤回，仅应用于 burn_after_read 的消息，避免误撤回。
func (s *MessageStore) RecallBySeq(ctx context.Context, convID string, seq int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE messages SET recalled=1 WHERE conv_id=? AND seq=? AND burn_after_read=1`, convID, seq)
	return err
}

// GetBySeq 查询会话内指定 seq 的消息元信息（用于判定阅后即焚等属性）。
func (s *MessageStore) GetBySeq(ctx context.Context, convID string, seq int64) (*models.Message, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT server_msg_id, client_msg_id, conv_id, conv_type, from_user_id, to_user_id, group_id, seq, timestamp, type, payload, recalled, expire_at, burn_after_read FROM messages WHERE conv_id=? AND seq=?`, convID, seq)
	m := &models.Message{}
	var nt sql.NullTime
	if err := row.Scan(&m.ServerMsgID, &m.ClientMsgID, &m.ConvID, &m.ConvType, &m.FromUserID, &m.ToUserID, &m.GroupID, &m.Seq, &m.Timestamp, &m.Type, &m.Payload, &m.Recalled, &nt, &m.BurnAfterRead); err != nil {
		return nil, err
	}
	if nt.Valid {
		t := nt.Time
		m.ExpireAt = &t
	}
	return m, nil
}
