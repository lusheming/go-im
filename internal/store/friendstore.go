package store

import (
	"context"
	"database/sql"
	"time"
)

// 好友关系存储
type FriendStore struct{ DB *sql.DB }

func NewFriendStore(db *sql.DB) *FriendStore { return &FriendStore{DB: db} }

// 添加或更新好友（备注）
func (s *FriendStore) AddFriend(ctx context.Context, userID, friendID, remark string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO friends(user_id, friend_id, remark, created_at, updated_at) VALUES(?,?,?,?,?) ON DUPLICATE KEY UPDATE remark=VALUES(remark), updated_at=VALUES(updated_at)`, userID, friendID, remark, time.Now(), time.Now())
	return err
}

// 修改好友备注
func (s *FriendStore) UpdateRemark(ctx context.Context, userID, friendID, remark string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE friends SET remark=?, updated_at=? WHERE user_id=? AND friend_id=?`, remark, time.Now(), userID, friendID)
	return err
}

// 删除好友
func (s *FriendStore) DeleteFriend(ctx context.Context, userID, friendID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM friends WHERE user_id=? AND friend_id=?`, userID, friendID)
	return err
}

// 是否为好友：放宽为单向即可（任一方向存在关系即认为允许发起会话）
func (s *FriendStore) IsFriend(ctx context.Context, a, b string) (bool, error) {
	var x int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM friends WHERE (user_id=? AND friend_id=?) OR (user_id=? AND friend_id=?)`, a, b, b, a).Scan(&x)
	if err != nil {
		return false, err
	}
	return x >= 1, nil
}

// 获取用户的好友列表
func (s *FriendStore) ListFriends(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			f.friend_id,
			f.remark,
			f.created_at,
			u.username,
			u.nickname,
			u.avatar_url
		FROM friends f
		LEFT JOIN users u ON f.friend_id = u.id
		WHERE f.user_id = ?
		ORDER BY f.created_at DESC
	`

	rows, err := s.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var friends []map[string]interface{}
	for rows.Next() {
		var friendID, remark, username, nickname, avatarURL sql.NullString
		var createdAt time.Time

		err := rows.Scan(&friendID, &remark, &createdAt, &username, &nickname, &avatarURL)
		if err != nil {
			continue
		}

		friend := map[string]interface{}{
			"id":        friendID.String,
			"username":  username.String,
			"nickname":  nickname.String,
			"avatarUrl": avatarURL.String,
			"remark":    remark.String,
			"createdAt": createdAt,
		}

		friends = append(friends, friend)
	}

	return friends, nil
}
