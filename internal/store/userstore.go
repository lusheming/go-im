package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go-im/internal/models"
)

// 用户存储
type UserStore struct{ DB *sql.DB }

func NewUserStore(db *sql.DB) *UserStore { return &UserStore{DB: db} }

// 创建用户
func (s *UserStore) CreateUser(ctx context.Context, u *models.User) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO users(id, username, password, nickname, avatar_url, created_at, updated_at) VALUES(?,?,?,?,?,NOW(),NOW())`, u.ID, u.Username, u.Password, u.Nickname, u.AvatarURL)
	return err
}

// 按用户名查询
func (s *UserStore) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT id, username, password, nickname, avatar_url, created_at, updated_at FROM users WHERE username=?`, username)
	u := &models.User{}
	if err := row.Scan(&u.ID, &u.Username, &u.Password, &u.Nickname, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

// 更新用户资料
func (s *UserStore) UpdateUser(ctx context.Context, u *models.User) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET nickname=?, avatar_url=?, updated_at=? WHERE id=?`, u.Nickname, u.AvatarURL, time.Now(), u.ID)
	return err
}

// 按 ID 查询用户
func (s *UserStore) GetByID(ctx context.Context, userID string) (*models.User, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT id, username, password, nickname, avatar_url, created_at, updated_at FROM users WHERE id=?`, userID)
	u := &models.User{}
	if err := row.Scan(&u.ID, &u.Username, &u.Password, &u.Nickname, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

// 统计用户总数
func (s *UserStore) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// 列出用户（分页）
func (s *UserStore) ListUsers(ctx context.Context, offset, limit int) ([]*models.User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, username, password, nickname, avatar_url, created_at, updated_at FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u := &models.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Password, &u.Nickname, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// 搜索用户（按用户名或昵称，排除自己和已是好友的用户）
func (s *UserStore) SearchUsers(ctx context.Context, query string, currentUserID string) ([]map[string]interface{}, error) {
	searchSQL := `
		SELECT 
			u.id,
			u.username,
			u.nickname,
			u.avatar_url,
			u.created_at,
			CASE WHEN f.friend_id IS NOT NULL THEN 1 ELSE 0 END as is_friend
		FROM users u
		LEFT JOIN friends f ON (f.user_id = ? AND f.friend_id = u.id)
		WHERE (u.username LIKE ? OR u.nickname LIKE ?) 
		  AND u.id != ?
		ORDER BY 
			CASE WHEN f.friend_id IS NOT NULL THEN 1 ELSE 0 END ASC,
			u.username ASC
		LIMIT 20
	`

	searchPattern := "%" + query + "%"
	rows, err := s.DB.QueryContext(ctx, searchSQL, currentUserID, searchPattern, searchPattern, currentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var userID, username, nickname, avatarURL string
		var createdAt time.Time
		var isFriend int

		err := rows.Scan(&userID, &username, &nickname, &avatarURL, &createdAt, &isFriend)
		if err != nil {
			continue
		}

		user := map[string]interface{}{
			"id":        userID,
			"username":  username,
			"nickname":  nickname,
			"avatarUrl": avatarURL,
			"createdAt": createdAt,
			"isFriend":  isFriend == 1,
		}

		users = append(users, user)
	}

	return users, nil
}
