package store

import (
	"context"
	"database/sql"
	"time"

	"go-im/internal/models"
)

// 群组与成员存储
type GroupStore struct{ DB *sql.DB }

func NewGroupStore(db *sql.DB) *GroupStore { return &GroupStore{DB: db} }

// 创建群组
func (s *GroupStore) CreateGroup(ctx context.Context, id, name, ownerID string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO `+"`groups`"+`(id, name, owner_id, muted, created_at, updated_at) VALUES(?,?,?,?,?,?)`, id, name, ownerID, 0, time.Now(), time.Now())
	return err
}

// 添加/更新成员
func (s *GroupStore) AddMember(ctx context.Context, groupID, userID, role, remark string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO group_members(group_id, user_id, role, remark, muted_until, created_at, updated_at) VALUES(?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE role=VALUES(role), remark=VALUES(remark), muted_until=VALUES(muted_until), updated_at=VALUES(updated_at)`, groupID, userID, role, remark, nil, time.Now(), time.Now())
	return err
}

// 移除成员
func (s *GroupStore) RemoveMember(ctx context.Context, groupID, userID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM group_members WHERE group_id=? AND user_id=?`, groupID, userID)
	return err
}

// 用户加入群（成员表插入为 member）
func (s *GroupStore) JoinGroup(ctx context.Context, groupID, userID string) error {
	return s.AddMember(ctx, groupID, userID, "member", "")
}

// 是否群成员
func (s *GroupStore) IsMember(ctx context.Context, groupID, userID string) (bool, error) {
	var x int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM group_members WHERE group_id=? AND user_id=?`, groupID, userID).Scan(&x)
	if err != nil {
		return false, err
	}
	return x > 0, nil
}

// 列出群所有成员 userId
func (s *GroupStore) ListMemberIDs(ctx context.Context, groupID string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id FROM group_members WHERE group_id=?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err == nil {
			ids = append(ids, uid)
		}
	}
	return ids, nil
}

// 获取用户的群组列表
func (s *GroupStore) ListUserGroups(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			g.id,
			g.name,
			g.owner_id,
			g.muted,
			g.created_at,
			gm.role,
			gm.remark as member_remark,
			gm.created_at as join_time,
			(SELECT COUNT(*) FROM group_members WHERE group_id = g.id) as member_count
		FROM group_members gm
		LEFT JOIN ` + "`groups`" + ` g ON gm.group_id = g.id
		WHERE gm.user_id = ?
		ORDER BY gm.created_at DESC
	`

	rows, err := s.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []map[string]interface{}
	for rows.Next() {
		var groupID, name, ownerID, role sql.NullString
		var memberRemark sql.NullString
		var muted, memberCount int
		var createdAt, joinTime time.Time

		err := rows.Scan(&groupID, &name, &ownerID, &muted, &createdAt, &role, &memberRemark, &joinTime, &memberCount)
		if err != nil {
			continue
		}

		group := map[string]interface{}{
			"id":          groupID.String,
			"name":        name.String,
			"ownerId":     ownerID.String,
			"muted":       muted == 1,
			"createdAt":   createdAt,
			"role":        role.String,
			"remark":      memberRemark.String,
			"joinTime":    joinTime,
			"memberCount": memberCount,
		}

		groups = append(groups, group)
	}

	return groups, nil
}

// 设置全员禁言
func (s *GroupStore) SetGroupMute(ctx context.Context, groupID string, mute bool) error {
	m := 0
	if mute {
		m = 1
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE `+"`groups`"+` SET muted=?, updated_at=? WHERE id=?`, m, time.Now(), groupID)
	return err
}

// 设置成员禁言截止时间
func (s *GroupStore) SetMemberMuteUntil(ctx context.Context, groupID, userID string, until *time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE group_members SET muted_until=?, updated_at=? WHERE group_id=? AND user_id=?`, until, time.Now(), groupID, userID)
	return err
}

// 查询是否被禁言（返回 true 表示当前不能发言）
func (s *GroupStore) IsMuted(ctx context.Context, groupID, userID string) (bool, error) {
	var groupMuted int
	_ = s.DB.QueryRowContext(ctx, `SELECT muted FROM `+"`groups`"+` WHERE id=?`, groupID).Scan(&groupMuted)
	if groupMuted == 1 {
		return true, nil
	}
	var until *time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT muted_until FROM group_members WHERE group_id=? AND user_id=?`, groupID, userID).Scan(&until)
	if err == nil && until != nil && until.After(time.Now()) {
		return true, nil
	}
	return false, nil
}

// 新增群公告
func (s *GroupStore) CreateNotice(ctx context.Context, id, groupID, title, content, createdBy string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO group_notices(id, group_id, title, content, created_by, created_at) VALUES(?,?,?,?,?,?)`, id, groupID, title, content, createdBy, time.Now())
	return err
}

// 列表公告（按时间倒序）
func (s *GroupStore) ListNotices(ctx context.Context, groupID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, title, content, created_by, created_at FROM group_notices WHERE group_id=? ORDER BY created_at DESC LIMIT ?`, groupID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, title, content, createdBy string
		var createdAt time.Time
		if err := rows.Scan(&id, &title, &content, &createdBy, &createdAt); err == nil {
			out = append(out, map[string]any{"id": id, "title": title, "content": content, "createdBy": createdBy, "createdAt": createdAt})
		}
	}
	return out, nil
}

// 统计群组总数
func (s *GroupStore) CountGroups(ctx context.Context) (int, error) {
	var count int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+"`groups`").Scan(&count)
	return count, err
}

// 列出群组（分页）
func (s *GroupStore) ListGroups(ctx context.Context, offset, limit int) ([]*models.Group, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, owner_id, created_at, updated_at FROM `+"`groups`"+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*models.Group
	for rows.Next() {
		g := &models.Group{}
		if err := rows.Scan(&g.ID, &g.Name, &g.OwnerID, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}
