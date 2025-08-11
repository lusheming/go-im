package external

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strings"
	"time"

	"go-im/internal/application/ports"
)

// IDGeneratorAdapter ID生成器适配器
type IDGeneratorAdapter struct{}

// NewIDGeneratorAdapter 创建ID生成器适配器
func NewIDGeneratorAdapter() ports.IDGenerator {
	return &IDGeneratorAdapter{}
}

// GenerateUserID 生成用户ID
func (g *IDGeneratorAdapter) GenerateUserID() string {
	return "user_" + g.generateRandomID()
}

// GenerateMessageID 生成消息ID
func (g *IDGeneratorAdapter) GenerateMessageID() string {
	return "msg_" + g.generateRandomID()
}

// GenerateGroupID 生成群组ID
func (g *IDGeneratorAdapter) GenerateGroupID() string {
	return "group_" + g.generateRandomID()
}

// GenerateConversationID 生成会话ID
func (g *IDGeneratorAdapter) GenerateConversationID(convType string, participants []string) string {
	if convType == "c2c" && len(participants) == 2 {
		// C2C会话：使用固定规则生成，确保相同用户的会话ID一致
		sorted := make([]string, len(participants))
		copy(sorted, participants)
		sort.Strings(sorted)
		return "conv_c2c_" + strings.Join(sorted, "_")
	}
	// 群聊会话：使用随机ID
	return "conv_" + convType + "_" + g.generateRandomID()
}

// generateRandomID 生成随机ID
func (g *IDGeneratorAdapter) generateRandomID() string {
	// 使用时间戳 + 随机数生成唯一ID
	timestamp := time.Now().UnixNano()

	// 生成8字节随机数
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)

	return fmt.Sprintf("%d_%x", timestamp, randomBytes)
}
