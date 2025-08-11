package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-im/internal/models"
	"go-im/internal/store/sqlstore"

	"github.com/google/uuid"
)

// 收藏服务
type FavoriteService struct {
	DB *sqlstore.Stores
}

func NewFavoriteService(db *sqlstore.Stores) *FavoriteService {
	return &FavoriteService{DB: db}
}

// 收藏消息
func (s *FavoriteService) FavoriteMessage(ctx context.Context, userID, messageID, convID string) (*models.Favorite, error) {
	// 检查是否已收藏
	if exists, _ := s.isFavoriteExists(ctx, userID, messageID); exists {
		return nil, fmt.Errorf("message already favorited")
	}

	// 获取原始消息内容（这里需要从消息服务获取）
	// 为简化，直接创建收藏记录
	favorite := &models.Favorite{
		ID:        uuid.NewString(),
		UserID:    userID,
		MessageID: messageID,
		ConvID:    convID,
		Type:      "message",
		Title:     "消息收藏",
		Content:   []byte(`{"messageId":"` + messageID + `"}`),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.createFavorite(ctx, favorite); err != nil {
		return nil, err
	}

	return favorite, nil
}

// 收藏自定义内容
func (s *FavoriteService) FavoriteCustom(ctx context.Context, userID, convID, title string, content interface{}, tags []string) (*models.Favorite, error) {
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content: %w", err)
	}

	favorite := &models.Favorite{
		ID:        uuid.NewString(),
		UserID:    userID,
		ConvID:    convID,
		Type:      "custom",
		Title:     title,
		Content:   contentBytes,
		Tags:      strings.Join(tags, ","),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.createFavorite(ctx, favorite); err != nil {
		return nil, err
	}

	return favorite, nil
}

// 删除收藏
func (s *FavoriteService) DeleteFavorite(ctx context.Context, favoriteID, userID string) error {
	query := `DELETE FROM favorites WHERE id = ? AND user_id = ?`
	result, err := s.DB.Primary.ExecContext(ctx, query, favoriteID, userID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("favorite not found or permission denied")
	}

	return nil
}

// 获取用户收藏列表
func (s *FavoriteService) ListFavorites(ctx context.Context, userID string, favoriteType string, tags string, limit, offset int) ([]*models.Favorite, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// 构建查询条件
	conditions := []string{"user_id = ?"}
	args := []interface{}{userID}

	if favoriteType != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, favoriteType)
	}

	if tags != "" {
		conditions = append(conditions, "tags LIKE ?")
		args = append(args, "%"+tags+"%")
	}

	query := fmt.Sprintf(`SELECT id, user_id, message_id, conv_id, type, title, content, tags, created_at, updated_at 
						  FROM favorites WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		strings.Join(conditions, " AND "))

	args = append(args, limit, offset)

	rows, err := s.DB.Primary.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favorites []*models.Favorite
	for rows.Next() {
		var favorite models.Favorite
		var messageID sql.NullString

		err := rows.Scan(&favorite.ID, &favorite.UserID, &messageID, &favorite.ConvID,
			&favorite.Type, &favorite.Title, &favorite.Content, &favorite.Tags,
			&favorite.CreatedAt, &favorite.UpdatedAt)
		if err != nil {
			continue
		}

		if messageID.Valid {
			favorite.MessageID = messageID.String
		}

		favorites = append(favorites, &favorite)
	}

	return favorites, nil
}

// 获取收藏详情
func (s *FavoriteService) GetFavorite(ctx context.Context, favoriteID, userID string) (*models.Favorite, error) {
	query := `SELECT id, user_id, message_id, conv_id, type, title, content, tags, created_at, updated_at 
			  FROM favorites WHERE id = ? AND user_id = ?`

	var favorite models.Favorite
	var messageID sql.NullString

	err := s.DB.Primary.QueryRowContext(ctx, query, favoriteID, userID).Scan(
		&favorite.ID, &favorite.UserID, &messageID, &favorite.ConvID,
		&favorite.Type, &favorite.Title, &favorite.Content, &favorite.Tags,
		&favorite.CreatedAt, &favorite.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("favorite not found")
	}

	if messageID.Valid {
		favorite.MessageID = messageID.String
	}

	return &favorite, nil
}

// 更新收藏标题和标签
func (s *FavoriteService) UpdateFavorite(ctx context.Context, favoriteID, userID, title, tags string) error {
	query := `UPDATE favorites SET title = ?, tags = ?, updated_at = ? WHERE id = ? AND user_id = ?`
	result, err := s.DB.Primary.ExecContext(ctx, query, title, tags, time.Now(), favoriteID, userID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("favorite not found or permission denied")
	}

	return nil
}

// 搜索收藏
func (s *FavoriteService) SearchFavorites(ctx context.Context, userID, keyword string, limit, offset int) ([]*models.Favorite, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := `SELECT id, user_id, message_id, conv_id, type, title, content, tags, created_at, updated_at 
			  FROM favorites WHERE user_id = ? AND (title LIKE ? OR tags LIKE ?) 
			  ORDER BY created_at DESC LIMIT ? OFFSET ?`

	searchPattern := "%" + keyword + "%"
	rows, err := s.DB.Primary.QueryContext(ctx, query, userID, searchPattern, searchPattern, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favorites []*models.Favorite
	for rows.Next() {
		var favorite models.Favorite
		var messageID sql.NullString

		err := rows.Scan(&favorite.ID, &favorite.UserID, &messageID, &favorite.ConvID,
			&favorite.Type, &favorite.Title, &favorite.Content, &favorite.Tags,
			&favorite.CreatedAt, &favorite.UpdatedAt)
		if err != nil {
			continue
		}

		if messageID.Valid {
			favorite.MessageID = messageID.String
		}

		favorites = append(favorites, &favorite)
	}

	return favorites, nil
}

// 获取收藏统计
func (s *FavoriteService) GetFavoriteStats(ctx context.Context, userID string) (map[string]int, error) {
	query := `SELECT type, COUNT(*) as count FROM favorites WHERE user_id = ? GROUP BY type`

	rows, err := s.DB.Primary.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var favoriteType string
		var count int
		if err := rows.Scan(&favoriteType, &count); err != nil {
			continue
		}
		stats[favoriteType] = count
	}

	// 获取总数
	totalQuery := `SELECT COUNT(*) FROM favorites WHERE user_id = ?`
	var total int
	s.DB.Primary.QueryRowContext(ctx, totalQuery, userID).Scan(&total)
	stats["total"] = total

	return stats, nil
}

// 检查收藏是否存在
func (s *FavoriteService) isFavoriteExists(ctx context.Context, userID, messageID string) (bool, error) {
	query := `SELECT COUNT(*) FROM favorites WHERE user_id = ? AND message_id = ?`
	var count int
	err := s.DB.Primary.QueryRowContext(ctx, query, userID, messageID).Scan(&count)
	return count > 0, err
}

// 创建收藏记录
func (s *FavoriteService) createFavorite(ctx context.Context, favorite *models.Favorite) error {
	query := `INSERT INTO favorites (id, user_id, message_id, conv_id, type, title, content, tags, created_at, updated_at) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.DB.Primary.ExecContext(ctx, query,
		favorite.ID, favorite.UserID, favorite.MessageID, favorite.ConvID,
		favorite.Type, favorite.Title, favorite.Content, favorite.Tags,
		favorite.CreatedAt, favorite.UpdatedAt)

	return err
}
