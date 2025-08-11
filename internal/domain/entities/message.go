package entities

import (
	"errors"
	"time"

	"go-im/internal/domain/valueobjects"
)

// Message 消息领域实体
// 包含消息的核心业务属性和行为，支持阅后即焚和定时自毁
type Message struct {
	serverMsgID string
	clientMsgID string
	convID      string
	convType    valueobjects.ConversationType
	fromUserID  string
	toUserID    string
	groupID     string
	seq         int64
	timestamp   time.Time
	msgType     string
	payload     []byte
	recalled    bool
	// 流式消息字段
	streamID     string
	streamSeq    int
	streamStatus string
	isStreaming  bool
	// 自毁/过期
	expireAt      *time.Time
	burnAfterRead bool
}

// NewMessage 创建新消息实体
func NewMessage(
	serverMsgID, clientMsgID, convID string,
	convType valueobjects.ConversationType,
	fromUserID string,
	seq int64,
	msgType string,
	payload []byte,
) (*Message, error) {
	if serverMsgID == "" {
		return nil, errors.New("服务器消息ID不能为空")
	}
	if clientMsgID == "" {
		return nil, errors.New("客户端消息ID不能为空")
	}
	if convID == "" {
		return nil, errors.New("会话ID不能为空")
	}
	if fromUserID == "" {
		return nil, errors.New("发送者ID不能为空")
	}
	if len(payload) == 0 {
		return nil, errors.New("消息内容不能为空")
	}

	return &Message{
		serverMsgID: serverMsgID,
		clientMsgID: clientMsgID,
		convID:      convID,
		convType:    convType,
		fromUserID:  fromUserID,
		seq:         seq,
		timestamp:   time.Now(),
		msgType:     msgType,
		payload:     payload,
		recalled:    false,
	}, nil
}

// ServerMsgID 获取服务器消息ID
func (m *Message) ServerMsgID() string {
	return m.serverMsgID
}

// ClientMsgID 获取客户端消息ID
func (m *Message) ClientMsgID() string {
	return m.clientMsgID
}

// ConvID 获取会话ID
func (m *Message) ConvID() string {
	return m.convID
}

// ConvType 获取会话类型
func (m *Message) ConvType() valueobjects.ConversationType {
	return m.convType
}

// FromUserID 获取发送者ID
func (m *Message) FromUserID() string {
	return m.fromUserID
}

// ToUserID 获取接收者ID
func (m *Message) ToUserID() string {
	return m.toUserID
}

// GroupID 获取群组ID
func (m *Message) GroupID() string {
	return m.groupID
}

// Seq 获取消息序号
func (m *Message) Seq() int64 {
	return m.seq
}

// Timestamp 获取时间戳
func (m *Message) Timestamp() time.Time {
	return m.timestamp
}

// MsgType 获取消息类型
func (m *Message) MsgType() string {
	return m.msgType
}

// Payload 获取消息内容
func (m *Message) Payload() []byte {
	return m.payload
}

// IsRecalled 是否已撤回
func (m *Message) IsRecalled() bool {
	return m.recalled
}

// ExpireAt 获取过期时间
func (m *Message) ExpireAt() *time.Time {
	return m.expireAt
}

// BurnAfterRead 是否阅后即焚
func (m *Message) BurnAfterRead() bool {
	return m.burnAfterRead
}

// SetToUserID 设置接收者ID（用于C2C消息）
func (m *Message) SetToUserID(toUserID string) {
	m.toUserID = toUserID
}

// SetGroupID 设置群组ID（用于群组消息）
func (m *Message) SetGroupID(groupID string) {
	m.groupID = groupID
}

// SetExpireAt 设置过期时间（定时自毁）
func (m *Message) SetExpireAt(expireAt *time.Time) {
	m.expireAt = expireAt
}

// SetBurnAfterRead 设置阅后即焚
func (m *Message) SetBurnAfterRead(burnAfterRead bool) {
	m.burnAfterRead = burnAfterRead
}

// Recall 撤回消息
func (m *Message) Recall() error {
	if m.recalled {
		return errors.New("消息已经被撤回")
	}
	m.recalled = true
	return nil
}

// IsExpired 检查消息是否已过期
func (m *Message) IsExpired(now time.Time) bool {
	return m.expireAt != nil && m.expireAt.Before(now)
}

// CanRecall 检查是否可以撤回（业务规则：5分钟内可撤回）
func (m *Message) CanRecall(now time.Time) bool {
	if m.recalled {
		return false
	}
	return now.Sub(m.timestamp) <= 5*time.Minute
}

// SetStreamInfo 设置流式消息信息
func (m *Message) SetStreamInfo(streamID string, streamSeq int, streamStatus string, isStreaming bool) {
	m.streamID = streamID
	m.streamSeq = streamSeq
	m.streamStatus = streamStatus
	m.isStreaming = isStreaming
}

// IsStreamMessage 是否为流式消息
func (m *Message) IsStreamMessage() bool {
	return m.isStreaming
}

// ToDTO 转换为数据传输对象
func (m *Message) ToDTO() MessageDTO {
	return MessageDTO{
		ServerMsgID:   m.serverMsgID,
		ClientMsgID:   m.clientMsgID,
		ConvID:        m.convID,
		ConvType:      string(m.convType),
		FromUserID:    m.fromUserID,
		ToUserID:      m.toUserID,
		GroupID:       m.groupID,
		Seq:           m.seq,
		Timestamp:     m.timestamp,
		Type:          m.msgType,
		Payload:       m.payload,
		Recalled:      m.recalled,
		StreamID:      m.streamID,
		StreamSeq:     m.streamSeq,
		StreamStatus:  m.streamStatus,
		IsStreaming:   m.isStreaming,
		ExpireAt:      m.expireAt,
		BurnAfterRead: m.burnAfterRead,
	}
}

// MessageDTO 消息数据传输对象
type MessageDTO struct {
	ServerMsgID   string     `json:"serverMsgId"`
	ClientMsgID   string     `json:"clientMsgId"`
	ConvID        string     `json:"convId"`
	ConvType      string     `json:"convType"`
	FromUserID    string     `json:"fromUserId"`
	ToUserID      string     `json:"toUserId,omitempty"`
	GroupID       string     `json:"groupId,omitempty"`
	Seq           int64      `json:"seq"`
	Timestamp     time.Time  `json:"timestamp"`
	Type          string     `json:"type"`
	Payload       []byte     `json:"payload"`
	Recalled      bool       `json:"recalled"`
	StreamID      string     `json:"streamId,omitempty"`
	StreamSeq     int        `json:"streamSeq,omitempty"`
	StreamStatus  string     `json:"streamStatus,omitempty"`
	IsStreaming   bool       `json:"isStreaming,omitempty"`
	ExpireAt      *time.Time `json:"expireAt,omitempty"`
	BurnAfterRead bool       `json:"burnAfterRead,omitempty"`
}

// FromMessageDTO 从DTO创建消息实体
func FromMessageDTO(dto MessageDTO) (*Message, error) {
	convType, err := valueobjects.NewConversationType(dto.ConvType)
	if err != nil {
		return nil, err
	}

	return &Message{
		serverMsgID:   dto.ServerMsgID,
		clientMsgID:   dto.ClientMsgID,
		convID:        dto.ConvID,
		convType:      convType,
		fromUserID:    dto.FromUserID,
		toUserID:      dto.ToUserID,
		groupID:       dto.GroupID,
		seq:           dto.Seq,
		timestamp:     dto.Timestamp,
		msgType:       dto.Type,
		payload:       dto.Payload,
		recalled:      dto.Recalled,
		streamID:      dto.StreamID,
		streamSeq:     dto.StreamSeq,
		streamStatus:  dto.StreamStatus,
		isStreaming:   dto.IsStreaming,
		expireAt:      dto.ExpireAt,
		burnAfterRead: dto.BurnAfterRead,
	}, nil
}
