package valueobjects

import "errors"

// ConversationType 会话类型值对象
// 值对象是不可变的，用于封装业务概念
type ConversationType string

const (
	ConversationTypeC2C   ConversationType = "c2c"
	ConversationTypeGroup ConversationType = "group"
)

// NewConversationType 创建会话类型值对象
func NewConversationType(value string) (ConversationType, error) {
	convType := ConversationType(value)
	if !convType.IsValid() {
		return "", errors.New("无效的会话类型")
	}
	return convType, nil
}

// IsValid 验证会话类型是否有效
func (ct ConversationType) IsValid() bool {
	switch ct {
	case ConversationTypeC2C, ConversationTypeGroup:
		return true
	default:
		return false
	}
}

// String 返回字符串表示
func (ct ConversationType) String() string {
	return string(ct)
}

// IsC2C 是否为单聊
func (ct ConversationType) IsC2C() bool {
	return ct == ConversationTypeC2C
}

// IsGroup 是否为群聊
func (ct ConversationType) IsGroup() bool {
	return ct == ConversationTypeGroup
}
