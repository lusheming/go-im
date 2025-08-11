package models

import "time"

// User/Group/Friend/Conversation/Message 等为核心领域模型。
// Message 增加自毁/过期字段以支持阅后即焚与定时自毁能力；
// Stream* 字段用于流式消息（例如 AI 生成/长文本分片输出）。

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	Nickname  string    `json:"nickname"`
	AvatarURL string    `json:"avatarUrl"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Online    bool      `json:"online,omitempty"` // 在线状态（管理后台使用）
}

type Friend struct {
	UserID    string    `json:"userId"`
	FriendID  string    `json:"friendId"`
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	OwnerID     string    `json:"ownerId"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	MemberCount int       `json:"memberCount,omitempty"` // 成员数量（管理后台使用）
}

type GroupMember struct {
	GroupID   string    `json:"groupId"`
	UserID    string    `json:"userId"`
	Role      string    `json:"role"` // owner, admin, member
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ConversationType string

const (
	ConversationTypeC2C   ConversationType = "c2c"
	ConversationTypeGroup ConversationType = "group"
)

type Conversation struct {
	ID         string           `json:"id"`
	Type       ConversationType `json:"type"`
	OwnerID    string           `json:"ownerId"`
	PeerID     string           `json:"peerId,omitempty"`
	GroupID    string           `json:"groupId,omitempty"`
	LastMsgSeq int64            `json:"lastMsgSeq"`
	UpdatedAt  time.Time        `json:"updatedAt"`
}

// Message 表示会话中的一条消息。
// - Seq 为会话内顺序（建议使用严格递增的会话内序列生成器）
// - ExpireAt 到期自动清理（SQL 由后台任务；Mongo 由 TTL 索引）
// - BurnAfterRead 阅后即焚（对端 read 后触发撤回并广播）
type Message struct {
	ServerMsgID string           `json:"serverMsgId"`
	ClientMsgID string           `json:"clientMsgId"`
	ConvID      string           `json:"convId"`
	ConvType    ConversationType `json:"convType"`
	FromUserID  string           `json:"fromUserId"`
	ToUserID    string           `json:"toUserId,omitempty"`
	GroupID     string           `json:"groupId,omitempty"`
	Seq         int64            `json:"seq"`
	Timestamp   time.Time        `json:"timestamp"`
	Type        string           `json:"type"`
	Payload     []byte           `json:"payload"`
	Recalled    bool             `json:"recalled"`
	// 流式消息字段
	StreamID     string `json:"streamId,omitempty"`     // 流式消息唯一标识
	StreamSeq    int    `json:"streamSeq,omitempty"`    // 流内序号（从1开始）
	StreamStatus string `json:"streamStatus,omitempty"` // start, chunk, end, error
	IsStreaming  bool   `json:"isStreaming,omitempty"`  // 是否为流式消息
	// 自毁/过期
	ExpireAt      *time.Time `json:"expireAt,omitempty"`      // 定时自毁时间（为空表示不过期）
	BurnAfterRead bool       `json:"burnAfterRead,omitempty"` // 阅后即焚（阅读后标记撤回）
}

type ReadReceipt struct {
	UserID string `json:"userId"`
	ConvID string `json:"convId"`
	Seq    int64  `json:"seq"`
	Time   int64  `json:"time"`
}

// 流式消息状态
const (
	StreamStatusStart = "start" // 开始流式输出
	StreamStatusChunk = "chunk" // 流式数据块
	StreamStatusEnd   = "end"   // 流式输出结束
	StreamStatusError = "error" // 流式输出错误
)

// 流式消息载荷
type StreamPayload struct {
	Text     string                 `json:"text,omitempty"`     // 文本内容（增量）
	Delta    string                 `json:"delta,omitempty"`    // 本次新增内容
	Metadata map[string]interface{} `json:"metadata,omitempty"` // 元数据
	Error    string                 `json:"error,omitempty"`    // 错误信息
}

// WebRTC 通话状态
const (
	CallStatusIdle     = "idle"     // 空闲
	CallStatusCalling  = "calling"  // 呼叫中
	CallStatusRinging  = "ringing"  // 响铃中
	CallStatusAnswered = "answered" // 已接听
	CallStatusEnded    = "ended"    // 已结束
	CallStatusRejected = "rejected" // 已拒接
	CallStatusBusy     = "busy"     // 忙线中
	CallStatusTimeout  = "timeout"  // 超时
)

// WebRTC 通话类型
const (
	CallTypeAudio = "audio" // 语音通话
	CallTypeVideo = "video" // 视频通话
)

// WebRTC 信令消息类型
const (
	SignalingTypeOffer     = "offer"     // SDP Offer
	SignalingTypeAnswer    = "answer"    // SDP Answer
	SignalingTypeCandidate = "candidate" // ICE Candidate
	SignalingTypeHangup    = "hangup"    // 挂断
)

// WebRTC 通话记录
type Call struct {
	ID         string     `json:"id"`                // 通话 ID
	FromUserID string     `json:"fromUserId"`        // 发起者
	ToUserID   string     `json:"toUserId"`          // 接收者
	Type       string     `json:"type"`              // audio/video
	Status     string     `json:"status"`            // 通话状态
	StartTime  time.Time  `json:"startTime"`         // 开始时间
	EndTime    *time.Time `json:"endTime,omitempty"` // 结束时间
	Duration   int64      `json:"duration"`          // 通话时长（秒）
}

// WebRTC 信令消息
type SignalingMessage struct {
	Type      string      `json:"type"`                // offer/answer/candidate/hangup
	CallID    string      `json:"callId"`              // 通话 ID
	From      string      `json:"from"`                // 发送者
	To        string      `json:"to"`                  // 接收者
	SDP       string      `json:"sdp,omitempty"`       // SDP 内容（offer/answer）
	Candidate interface{} `json:"candidate,omitempty"` // ICE Candidate
}

// 消息类型常量
const (
	MessageTypeText     = "text"     // 文本消息
	MessageTypeImage    = "image"    // 图片消息
	MessageTypeVoice    = "voice"    // 语音消息
	MessageTypeVideo    = "video"    // 视频消息
	MessageTypeFile     = "file"     // 文件消息
	MessageTypeCard     = "card"     // 名片消息
	MessageTypeLocation = "location" // 位置消息
	MessageTypeCustom   = "custom"   // 自定义消息
	MessageTypeStream   = "stream"   // 流式消息
)

// 文本消息载荷
type TextPayload struct {
	Text string `json:"text"` // 文本内容
}

// 图片消息载荷
type ImagePayload struct {
	URL       string `json:"url"`                 // 图片 URL
	Width     int    `json:"width,omitempty"`     // 图片宽度
	Height    int    `json:"height,omitempty"`    // 图片高度
	Size      int64  `json:"size,omitempty"`      // 文件大小（字节）
	Thumbnail string `json:"thumbnail,omitempty"` // 缩略图 URL
	Format    string `json:"format,omitempty"`    // 图片格式（jpg, png, gif, webp）
}

// 语音消息载荷
type VoicePayload struct {
	URL      string `json:"url"`                // 语音文件 URL
	Duration int    `json:"duration"`           // 语音时长（秒）
	Size     int64  `json:"size,omitempty"`     // 文件大小（字节）
	Format   string `json:"format,omitempty"`   // 音频格式（mp3, wav, m4a）
	Waveform string `json:"waveform,omitempty"` // 波形数据（可选）
}

// 视频消息载荷
type VideoPayload struct {
	URL       string `json:"url"`                 // 视频 URL
	Duration  int    `json:"duration"`            // 视频时长（秒）
	Width     int    `json:"width,omitempty"`     // 视频宽度
	Height    int    `json:"height,omitempty"`    // 视频高度
	Size      int64  `json:"size,omitempty"`      // 文件大小（字节）
	Thumbnail string `json:"thumbnail,omitempty"` // 视频封面 URL
	Format    string `json:"format,omitempty"`    // 视频格式（mp4, mov, avi）
}

// 文件消息载荷
type FilePayload struct {
	URL       string `json:"url"`                 // 文件 URL
	Name      string `json:"name"`                // 文件名
	Size      int64  `json:"size"`                // 文件大小（字节）
	MimeType  string `json:"mimeType"`            // MIME 类型
	Extension string `json:"extension,omitempty"` // 文件扩展名
}

// 名片消息载荷
type CardPayload struct {
	UserID   string `json:"userId"`          // 用户 ID
	Nickname string `json:"nickname"`        // 用户昵称
	Avatar   string `json:"avatar"`          // 用户头像
	Phone    string `json:"phone,omitempty"` // 电话号码（可选）
	Email    string `json:"email,omitempty"` // 邮箱（可选）
}

// 位置消息载荷
type LocationPayload struct {
	Latitude  float64 `json:"latitude"`        // 纬度
	Longitude float64 `json:"longitude"`       // 经度
	Address   string  `json:"address"`         // 地址描述
	Title     string  `json:"title,omitempty"` // 位置标题
}

// 自定义消息载荷
type CustomPayload struct {
	Type string                 `json:"type"` // 自定义类型
	Data map[string]interface{} `json:"data"` // 自定义数据
}

// 收藏消息
type Favorite struct {
	ID        string    `json:"id" db:"id"`                // 收藏 ID
	UserID    string    `json:"userId" db:"user_id"`       // 收藏者 ID
	MessageID string    `json:"messageId" db:"message_id"` // 消息 ID（可选）
	ConvID    string    `json:"convId" db:"conv_id"`       // 会话 ID
	Type      string    `json:"type" db:"type"`            // 收藏类型：message/custom
	Title     string    `json:"title" db:"title"`          // 收藏标题
	Content   []byte    `json:"content" db:"content"`      // 收藏内容（JSON）
	Tags      string    `json:"tags" db:"tags"`            // 标签（逗号分隔）
	CreatedAt time.Time `json:"createdAt" db:"created_at"` // 收藏时间
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"` // 更新时间
}

// 文件上传记录
type FileUpload struct {
	ID        string     `json:"id" db:"id"`                // 文件 ID
	UserID    string     `json:"userId" db:"user_id"`       // 上传者 ID
	FileName  string     `json:"fileName" db:"file_name"`   // 原始文件名
	FileSize  int64      `json:"fileSize" db:"file_size"`   // 文件大小
	MimeType  string     `json:"mimeType" db:"mime_type"`   // MIME 类型
	StorePath string     `json:"storePath" db:"store_path"` // 存储路径
	URL       string     `json:"url" db:"url"`              // 访问 URL
	Status    string     `json:"status" db:"status"`        // 状态：uploading/success/failed
	CreatedAt time.Time  `json:"createdAt" db:"created_at"` // 上传时间
	ExpiresAt *time.Time `json:"expiresAt" db:"expires_at"` // 过期时间（可选）
}
