package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go-im/internal/application/ports"
	"go-im/internal/domain/entities"
)

// UserRepositoryAdapter 用户仓储适配器
// 实现 ports.UserRepository 接口，提供用户数据访问
type UserRepositoryAdapter struct {
	db *sql.DB
}

// NewUserRepositoryAdapter 创建用户仓储适配器
func NewUserRepositoryAdapter(db *sql.DB) ports.UserRepository {
	return &UserRepositoryAdapter{db: db}
}

// Save 保存用户
func (r *UserRepositoryAdapter) Save(ctx context.Context, user *entities.User) error {
	query := `INSERT INTO users(id, username, password, nickname, avatar_url, created_at, updated_at) 
			  VALUES(?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		user.ID(),
		user.Username(),
		user.Password(),
		user.Nickname(),
		user.AvatarURL(),
		user.CreatedAt(),
		user.UpdatedAt(),
	)

	return err
}

// GetByID 根据ID获取用户
func (r *UserRepositoryAdapter) GetByID(ctx context.Context, id string) (*entities.User, error) {
	query := `SELECT id, username, password, nickname, avatar_url, created_at, updated_at 
			  FROM users WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)

	var userID, username, password, nickname, avatarURL string
	var createdAt, updatedAt time.Time

	err := row.Scan(&userID, &username, &password, &nickname, &avatarURL, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	// 使用 FromUserDTO 重建用户实体
	dto := entities.UserDTO{
		ID:        userID,
		Username:  username,
		Nickname:  nickname,
		AvatarURL: avatarURL,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	return entities.FromUserDTO(dto, password), nil
}

// GetByUsername 根据用户名获取用户
func (r *UserRepositoryAdapter) GetByUsername(ctx context.Context, username string) (*entities.User, error) {
	query := `SELECT id, username, password, nickname, avatar_url, created_at, updated_at 
			  FROM users WHERE username = ?`

	row := r.db.QueryRowContext(ctx, query, username)

	var userID, userUsername, password, nickname, avatarURL string
	var createdAt, updatedAt time.Time

	err := row.Scan(&userID, &userUsername, &password, &nickname, &avatarURL, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	dto := entities.UserDTO{
		ID:        userID,
		Username:  userUsername,
		Nickname:  nickname,
		AvatarURL: avatarURL,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	return entities.FromUserDTO(dto, password), nil
}

// Update 更新用户信息
func (r *UserRepositoryAdapter) Update(ctx context.Context, user *entities.User) error {
	query := `UPDATE users SET nickname = ?, avatar_url = ?, updated_at = ? WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query,
		user.Nickname(),
		user.AvatarURL(),
		user.UpdatedAt(),
		user.ID(),
	)

	return err
}

// Delete 删除用户
func (r *UserRepositoryAdapter) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM users WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// List 分页获取用户列表
func (r *UserRepositoryAdapter) List(ctx context.Context, offset, limit int) ([]*entities.User, error) {
	query := `SELECT id, username, password, nickname, avatar_url, created_at, updated_at 
			  FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*entities.User
	for rows.Next() {
		var userID, username, password, nickname, avatarURL string
		var createdAt, updatedAt time.Time

		err := rows.Scan(&userID, &username, &password, &nickname, &avatarURL, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}

		dto := entities.UserDTO{
			ID:        userID,
			Username:  username,
			Nickname:  nickname,
			AvatarURL: avatarURL,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}

		user := entities.FromUserDTO(dto, password)
		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// Count 获取用户总数
func (r *UserRepositoryAdapter) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM users`

	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}
