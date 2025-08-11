package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"go-im/internal/cache"
)

// 会话存储
type ConversationStore struct{ DB *sql.DB }

func NewConversationStore(db *sql.DB) *ConversationStore { return &ConversationStore{DB: db} }

// 更新会话最新 seq 与时间
func (s *ConversationStore) UpsertConversation(ctx context.Context, convID, convType, peerID, groupID string, lastSeq int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO conversations(id, conv_type, peer_id, group_id, last_seq, updated_at) VALUES(?,?,?,?,?,?) ON DUPLICATE KEY UPDATE last_seq=IF(VALUES(last_seq)>last_seq, VALUES(last_seq), last_seq), updated_at=VALUES(updated_at)`, convID, convType, peerID, groupID, lastSeq, time.Now())
	return err
}

// 建立用户与会话关系（用于列表），若存在则仅更新时间
func (s *ConversationStore) UpsertUserConversation(ctx context.Context, userID, convID, convType, peerID, groupID string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO user_conversations(user_id, conv_id, conv_type, peer_id, group_id, updated_at) VALUES(?,?,?,?,?,?) ON DUPLICATE KEY UPDATE updated_at=VALUES(updated_at)`, userID, convID, convType, peerID, groupID, time.Now())
	return err
}

// 按用户拉取会话列表（按更新时间倒序）
func (s *ConversationStore) ListByUser(ctx context.Context, userID string, limit int) (*sql.Rows, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.DB.QueryContext(ctx, `SELECT conv_id, conv_type, peer_id, group_id, pinned, muted, draft, updated_at FROM user_conversations WHERE user_id=? ORDER BY updated_at DESC LIMIT ?`, userID, limit)
}

// 获取会话 last_seq
func (s *ConversationStore) GetConversationLastSeq(ctx context.Context, convID string) (int64, error) {
	var seq sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT last_seq FROM conversations WHERE id=?`, convID).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if seq.Valid {
		return seq.Int64, nil
	}
	return 0, nil
}

func lastSeqCacheKey(convID string) string { return fmt.Sprintf("im:lastseq:%s", convID) }
func readSeqCacheKey(userID, convID string) string {
	return fmt.Sprintf("im:readseq:%s:%s", userID, convID)
}

// 带未读数的会话列表（未读=last_seq - read_seq，最小为0）
func (s *ConversationStore) ListWithUnread(ctx context.Context, userID string, limit int, receipt *ReceiptStore) ([]map[string]interface{}, error) {
	rows, err := s.ListByUser(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 收集 convIds 以便批量从 Redis 获取
	type rowData struct {
		convID, convType, peerID, groupID string
		pinned, muted                     int
		draft                             sql.NullString
		updatedAt                         time.Time
	}
	var items []rowData
	for rows.Next() {
		var r rowData
		if err := rows.Scan(&r.convID, &r.convType, &r.peerID, &r.groupID, &r.pinned, &r.muted, &r.draft, &r.updatedAt); err != nil {
			return nil, err
		}
		items = append(items, r)
	}

	pipe := cache.Client().Pipeline()
	lastSeqCmds := make([]*stringCmdWrapper, len(items))
	readSeqCmds := make([]*stringCmdWrapper, len(items))
	for i, it := range items {
		lastSeqCmds[i] = wrapStringCmd(pipe.Get(ctx, lastSeqCacheKey(it.convID)))
		readSeqCmds[i] = wrapStringCmd(pipe.Get(ctx, readSeqCacheKey(userID, it.convID)))
	}
	_, _ = pipe.Exec(ctx)

	var list []map[string]interface{}
	for i, it := range items {
		lastSeq, ok := parseInt64(lastSeqCmds[i])
		if !ok {
			lastSeq, _ = s.GetConversationLastSeq(ctx, it.convID)
			cache.Client().Set(ctx, lastSeqCacheKey(it.convID), lastSeq, 10*time.Minute)
		}
		readSeq, ok := parseInt64(readSeqCmds[i])
		if !ok {
			readSeq, _ = receipt.GetReadSeq(ctx, userID, it.convID)
			cache.Client().Set(ctx, readSeqCacheKey(userID, it.convID), readSeq, 10*time.Minute)
		}
		unread := lastSeq - readSeq
		if unread < 0 {
			unread = 0
		}
		list = append(list, map[string]interface{}{
			"convId":   it.convID,
			"convType": it.convType,
			"peerId":   it.peerID,
			"groupId":  it.groupID,
			"pinned":   it.pinned == 1,
			"muted":    it.muted == 1,
			"draft": func() string {
				if it.draft.Valid {
					return it.draft.String
				}
				return ""
			}(),
			"updatedAt": it.updatedAt,
			"lastSeq":   lastSeq,
			"readSeq":   readSeq,
			"unread":    unread,
		})
	}
	return list, nil
}

// 轻量封装以避免直接暴露 redis 依赖
type stringCmdWrapper struct{ res func() (string, error) }

func wrapStringCmd(cmd interface{ Result() (string, error) }) *stringCmdWrapper {
	return &stringCmdWrapper{res: cmd.Result}
}
func parseInt64(c *stringCmdWrapper) (int64, bool) {
	if c == nil || c.res == nil {
		return 0, false
	}
	s, err := c.res()
	if err != nil || s == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// 设置置顶
func (s *ConversationStore) SetPinned(ctx context.Context, userID, convID string, pinned bool) error {
	v := 0
	if pinned {
		v = 1
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE user_conversations SET pinned=?, updated_at=? WHERE user_id=? AND conv_id=?`, v, time.Now(), userID, convID)
	return err
}

// 设置免打扰
func (s *ConversationStore) SetMuted(ctx context.Context, userID, convID string, muted bool) error {
	v := 0
	if muted {
		v = 1
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE user_conversations SET muted=?, updated_at=? WHERE user_id=? AND conv_id=?`, v, time.Now(), userID, convID)
	return err
}

// 设置草稿
func (s *ConversationStore) SetDraft(ctx context.Context, userID, convID, draft string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE user_conversations SET draft=?, updated_at=? WHERE user_id=? AND conv_id=?`, draft, time.Now(), userID, convID)
	return err
}
