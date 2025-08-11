package ports

import (
	"context"
	"time"
)

// CacheService 缓存服务端口
// 定义缓存操作的抽象接口
type CacheService interface {
	// Set 设置缓存
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	// Get 获取缓存
	Get(ctx context.Context, key string) (string, error)
	// Delete 删除缓存
	Delete(ctx context.Context, key string) error
	// Exists 检查key是否存在
	Exists(ctx context.Context, key string) (bool, error)
	// SetNX 仅当key不存在时设置
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
}

// MessageBroker 消息代理端口
// 定义消息发布订阅的抽象接口
type MessageBroker interface {
	// Publish 发布消息
	Publish(ctx context.Context, topic string, message []byte) error
	// Subscribe 订阅消息
	Subscribe(ctx context.Context, topic string, handler func([]byte) error) error
	// Close 关闭连接
	Close() error
}

// PresenceService 在线状态服务端口
type PresenceService interface {
	// SetUserOnline 设置用户在线
	SetUserOnline(ctx context.Context, userID, deviceID string) error
	// SetUserOffline 设置用户离线
	SetUserOffline(ctx context.Context, userID, deviceID string) error
	// IsUserOnline 检查用户是否在线
	IsUserOnline(ctx context.Context, userID string) (bool, error)
	// GetOnlineUsers 获取在线用户列表
	GetOnlineUsers(ctx context.Context) ([]string, error)
	// GetUserDevices 获取用户在线设备
	GetUserDevices(ctx context.Context, userID string) ([]string, error)
}

// NotificationService 通知服务端口
type NotificationService interface {
	// SendToUser 向用户发送通知
	SendToUser(ctx context.Context, userID string, notification *Notification) error
	// SendToGroup 向群组发送通知
	SendToGroup(ctx context.Context, groupID string, notification *Notification) error
	// SendToUsers 向多个用户发送通知
	SendToUsers(ctx context.Context, userIDs []string, notification *Notification) error
}

// Notification 通知对象
type Notification struct {
	Type    string                 `json:"type"`
	Title   string                 `json:"title,omitempty"`
	Content string                 `json:"content,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// FileService 文件服务端口
type FileService interface {
	// Upload 上传文件
	Upload(ctx context.Context, filename string, data []byte) (*FileInfo, error)
	// Download 下载文件
	Download(ctx context.Context, fileID string) ([]byte, error)
	// Delete 删除文件
	Delete(ctx context.Context, fileID string) error
	// GetUploadURL 获取上传URL（用于直传）
	GetUploadURL(ctx context.Context, filename string, size int64) (*UploadInfo, error)
}

// FileInfo 文件信息
type FileInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
	MimeType string `json:"mimeType"`
}

// UploadInfo 上传信息
type UploadInfo struct {
	UploadURL string            `json:"uploadUrl"`
	Headers   map[string]string `json:"headers,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
	FileURL   string            `json:"fileUrl"`
}

// AuthService 认证服务端口
type AuthService interface {
	// GenerateToken 生成token
	GenerateToken(ctx context.Context, userID string, expiration time.Duration) (string, error)
	// ValidateToken 验证token
	ValidateToken(ctx context.Context, token string) (*TokenClaims, error)
	// RefreshToken 刷新token
	RefreshToken(ctx context.Context, token string) (string, error)
	// RevokeToken 撤销token
	RevokeToken(ctx context.Context, token string) error
}

// TokenClaims token声明
type TokenClaims struct {
	UserID    string    `json:"userId"`
	IssuedAt  time.Time `json:"issuedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// PasswordService 密码服务端口
type PasswordService interface {
	// HashPassword 加密密码
	HashPassword(password string) (string, error)
	// VerifyPassword 验证密码
	VerifyPassword(hashedPassword, password string) bool
}

// IDGenerator ID生成器端口
type IDGenerator interface {
	// GenerateUserID 生成用户ID
	GenerateUserID() string
	// GenerateMessageID 生成消息ID
	GenerateMessageID() string
	// GenerateGroupID 生成群组ID
	GenerateGroupID() string
	// GenerateConversationID 生成会话ID
	GenerateConversationID(convType string, participants []string) string
}

// SequenceGenerator 序列号生成器端口
type SequenceGenerator interface {
	// NextSeq 获取下一个序列号
	NextSeq(ctx context.Context, convID string) (int64, error)
	// CurrentSeq 获取当前序列号
	CurrentSeq(ctx context.Context, convID string) (int64, error)
}

// RateLimiter 限流器端口
type RateLimiter interface {
	// Allow 检查是否允许请求
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
	// Reset 重置限流计数
	Reset(ctx context.Context, key string) error
}

// MetricsService 指标服务端口
type MetricsService interface {
	// IncrementCounter 增加计数器
	IncrementCounter(name string, labels map[string]string)
	// RecordHistogram 记录直方图
	RecordHistogram(name string, value float64, labels map[string]string)
	// SetGauge 设置仪表盘
	SetGauge(name string, value float64, labels map[string]string)
}

// LogService 日志服务端口
type LogService interface {
	// Info 记录信息日志
	Info(ctx context.Context, message string, fields map[string]interface{})
	// Error 记录错误日志
	Error(ctx context.Context, message string, err error, fields map[string]interface{})
	// Warn 记录警告日志
	Warn(ctx context.Context, message string, fields map[string]interface{})
	// Debug 记录调试日志
	Debug(ctx context.Context, message string, fields map[string]interface{})
}
