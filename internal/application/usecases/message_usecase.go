package usecases

import (
	"context"
	"errors"
	"time"

	"go-im/internal/application/ports"
	"go-im/internal/domain/entities"
	"go-im/internal/domain/valueobjects"
)

// MessageUseCase 消息用例
// 实现消息相关的核心业务逻辑
type MessageUseCase struct {
	messageRepo       ports.MessageRepository
	userRepo          ports.UserRepository
	groupRepo         ports.GroupRepository
	groupMemberRepo   ports.GroupMemberRepository
	idGenerator       ports.IDGenerator
	sequenceGenerator ports.SequenceGenerator
	notificationSvc   ports.NotificationService
	presenceSvc       ports.PresenceService
	rateLimiter       ports.RateLimiter
	metricsSvc        ports.MetricsService
	logger            ports.LogService
}

// NewMessageUseCase 创建消息用例
func NewMessageUseCase(
	messageRepo ports.MessageRepository,
	userRepo ports.UserRepository,
	groupRepo ports.GroupRepository,
	groupMemberRepo ports.GroupMemberRepository,
	idGenerator ports.IDGenerator,
	sequenceGenerator ports.SequenceGenerator,
	notificationSvc ports.NotificationService,
	presenceSvc ports.PresenceService,
	rateLimiter ports.RateLimiter,
	metricsSvc ports.MetricsService,
	logger ports.LogService,
) *MessageUseCase {
	return &MessageUseCase{
		messageRepo:       messageRepo,
		userRepo:          userRepo,
		groupRepo:         groupRepo,
		groupMemberRepo:   groupMemberRepo,
		idGenerator:       idGenerator,
		sequenceGenerator: sequenceGenerator,
		notificationSvc:   notificationSvc,
		presenceSvc:       presenceSvc,
		rateLimiter:       rateLimiter,
		metricsSvc:        metricsSvc,
		logger:            logger,
	}
}

// SendMessageRequest 发送消息请求
type SendMessageRequest struct {
	ClientMsgID   string                        `json:"clientMsgId"`
	ConvID        string                        `json:"convId"`
	ConvType      valueobjects.ConversationType `json:"convType"`
	FromUserID    string                        `json:"fromUserId"`
	ToUserID      string                        `json:"toUserId,omitempty"`
	GroupID       string                        `json:"groupId,omitempty"`
	MsgType       string                        `json:"type"`
	Payload       []byte                        `json:"payload"`
	ExpireAt      *time.Time                    `json:"expireAt,omitempty"`
	BurnAfterRead bool                          `json:"burnAfterRead,omitempty"`
}

// SendMessageResponse 发送消息响应
type SendMessageResponse struct {
	ServerMsgID string    `json:"serverMsgId"`
	Seq         int64     `json:"seq"`
	Timestamp   time.Time `json:"timestamp"`
}

// SendMessage 发送消息
func (uc *MessageUseCase) SendMessage(ctx context.Context, req *SendMessageRequest) (*SendMessageResponse, error) {
	// 验证请求参数
	if err := uc.validateSendRequest(ctx, req); err != nil {
		return nil, err
	}

	// 限流检查
	rateLimitKey := "send_msg:" + req.FromUserID
	allowed, err := uc.rateLimiter.Allow(ctx, rateLimitKey, 100, time.Minute)
	if err != nil {
		uc.logger.Error(ctx, "限流检查失败", err, map[string]interface{}{
			"userId": req.FromUserID,
		})
		return nil, errors.New("系统繁忙，请稍后重试")
	}
	if !allowed {
		return nil, errors.New("发送消息过于频繁，请稍后重试")
	}

	// 生成消息ID和序列号
	serverMsgID := uc.idGenerator.GenerateMessageID()
	seq, err := uc.sequenceGenerator.NextSeq(ctx, req.ConvID)
	if err != nil {
		uc.logger.Error(ctx, "获取消息序列号失败", err, map[string]interface{}{
			"convId": req.ConvID,
		})
		return nil, errors.New("消息发送失败")
	}

	// 创建消息实体
	message, err := entities.NewMessage(
		serverMsgID,
		req.ClientMsgID,
		req.ConvID,
		req.ConvType,
		req.FromUserID,
		seq,
		req.MsgType,
		req.Payload,
	)
	if err != nil {
		return nil, err
	}

	// 设置消息属性
	if req.ConvType.IsC2C() {
		message.SetToUserID(req.ToUserID)
	} else if req.ConvType.IsGroup() {
		message.SetGroupID(req.GroupID)
	}

	if req.ExpireAt != nil {
		message.SetExpireAt(req.ExpireAt)
	}
	message.SetBurnAfterRead(req.BurnAfterRead)

	// 保存消息
	if err := uc.messageRepo.Save(ctx, message); err != nil {
		uc.logger.Error(ctx, "保存消息失败", err, map[string]interface{}{
			"serverMsgId": serverMsgID,
			"convId":      req.ConvID,
		})
		return nil, errors.New("消息发送失败")
	}

	// 发送通知
	if err := uc.sendNotification(ctx, message); err != nil {
		uc.logger.Error(ctx, "发送消息通知失败", err, map[string]interface{}{
			"serverMsgId": serverMsgID,
			"convId":      req.ConvID,
		})
		// 通知失败不影响消息发送成功
	}

	// 记录指标
	uc.metricsSvc.IncrementCounter("messages_sent_total", map[string]string{
		"conv_type": string(req.ConvType),
		"msg_type":  req.MsgType,
	})

	return &SendMessageResponse{
		ServerMsgID: serverMsgID,
		Seq:         seq,
		Timestamp:   message.Timestamp(),
	}, nil
}

// GetMessagesRequest 获取消息列表请求
type GetMessagesRequest struct {
	ConvID  string `json:"convId"`
	FromSeq int64  `json:"fromSeq"`
	Limit   int    `json:"limit"`
	UserID  string `json:"userId"` // 请求用户ID（用于权限验证）
}

// GetMessages 获取消息列表
func (uc *MessageUseCase) GetMessages(ctx context.Context, req *GetMessagesRequest) ([]*entities.MessageDTO, error) {
	// 验证权限
	if err := uc.validateGetMessagesPermission(ctx, req.UserID, req.ConvID); err != nil {
		return nil, err
	}

	// 获取消息列表
	messages, err := uc.messageRepo.List(ctx, req.ConvID, req.FromSeq, req.Limit)
	if err != nil {
		uc.logger.Error(ctx, "获取消息列表失败", err, map[string]interface{}{
			"convId":  req.ConvID,
			"fromSeq": req.FromSeq,
			"limit":   req.Limit,
		})
		return nil, errors.New("获取消息失败")
	}

	// 转换为DTO
	var result []*entities.MessageDTO
	for _, msg := range messages {
		dto := msg.ToDTO()
		result = append(result, &dto)
	}

	return result, nil
}

// RecallMessageRequest 撤回消息请求
type RecallMessageRequest struct {
	ServerMsgID string `json:"serverMsgId"`
	UserID      string `json:"userId"` // 操作用户ID
}

// RecallMessage 撤回消息
func (uc *MessageUseCase) RecallMessage(ctx context.Context, req *RecallMessageRequest) error {
	// 获取消息
	message, err := uc.messageRepo.GetByID(ctx, req.ServerMsgID)
	if err != nil {
		return errors.New("消息不存在")
	}

	// 验证权限（只有发送者可以撤回）
	if message.FromUserID() != req.UserID {
		return errors.New("只能撤回自己发送的消息")
	}

	// 检查是否可以撤回
	if !message.CanRecall(time.Now()) {
		return errors.New("消息发送超过5分钟，无法撤回")
	}

	// 撤回消息
	if err := message.Recall(); err != nil {
		return err
	}

	// 更新到仓储
	if err := uc.messageRepo.Recall(ctx, req.ServerMsgID); err != nil {
		uc.logger.Error(ctx, "撤回消息失败", err, map[string]interface{}{
			"serverMsgId": req.ServerMsgID,
		})
		return errors.New("撤回消息失败")
	}

	// 发送撤回通知
	if err := uc.sendRecallNotification(ctx, message); err != nil {
		uc.logger.Error(ctx, "发送撤回通知失败", err, map[string]interface{}{
			"serverMsgId": req.ServerMsgID,
		})
	}

	return nil
}

// MarkAsReadRequest 标记已读请求
type MarkAsReadRequest struct {
	ConvID string `json:"convId"`
	Seq    int64  `json:"seq"`
	UserID string `json:"userId"`
}

// MarkAsRead 标记消息已读（阅后即焚处理）
func (uc *MessageUseCase) MarkAsRead(ctx context.Context, req *MarkAsReadRequest) error {
	// 获取消息
	message, err := uc.messageRepo.GetBySeq(ctx, req.ConvID, req.Seq)
	if err != nil {
		return errors.New("消息不存在")
	}

	// 如果是阅后即焚消息且不是发送者，则标记为撤回
	if message.BurnAfterRead() && message.FromUserID() != req.UserID && !message.IsRecalled() {
		if err := uc.messageRepo.RecallBySeq(ctx, req.ConvID, req.Seq); err != nil {
			uc.logger.Error(ctx, "阅后即焚消息撤回失败", err, map[string]interface{}{
				"convId": req.ConvID,
				"seq":    req.Seq,
			})
			return err
		}

		// 发送撤回通知
		if err := uc.sendRecallNotification(ctx, message); err != nil {
			uc.logger.Error(ctx, "发送阅后即焚撤回通知失败", err, map[string]interface{}{
				"convId": req.ConvID,
				"seq":    req.Seq,
			})
		}
	}

	return nil
}

// CleanExpiredMessages 清理过期消息
func (uc *MessageUseCase) CleanExpiredMessages(ctx context.Context) error {
	now := time.Now()
	if err := uc.messageRepo.DeleteExpired(ctx, now); err != nil {
		uc.logger.Error(ctx, "清理过期消息失败", err, nil)
		return err
	}

	uc.logger.Info(ctx, "清理过期消息完成", map[string]interface{}{
		"cleanTime": now,
	})
	return nil
}

// validateSendRequest 验证发送消息请求
func (uc *MessageUseCase) validateSendRequest(ctx context.Context, req *SendMessageRequest) error {
	// 验证用户是否存在
	user, err := uc.userRepo.GetByID(ctx, req.FromUserID)
	if err != nil || user == nil {
		return errors.New("发送者不存在")
	}

	// 验证会话类型
	if req.ConvType.IsC2C() {
		if req.ToUserID == "" {
			return errors.New("单聊消息必须指定接收者")
		}
		// 验证接收者是否存在
		toUser, err := uc.userRepo.GetByID(ctx, req.ToUserID)
		if err != nil || toUser == nil {
			return errors.New("接收者不存在")
		}
	} else if req.ConvType.IsGroup() {
		if req.GroupID == "" {
			return errors.New("群聊消息必须指定群组")
		}
		// 验证群组是否存在
		group, err := uc.groupRepo.GetByID(ctx, req.GroupID)
		if err != nil || group == nil {
			return errors.New("群组不存在")
		}
		// 验证是否为群成员
		isMember, err := uc.groupMemberRepo.IsMember(ctx, req.GroupID, req.FromUserID)
		if err != nil || !isMember {
			return errors.New("非群成员无法发送消息")
		}
	}

	return nil
}

// validateGetMessagesPermission 验证获取消息权限
func (uc *MessageUseCase) validateGetMessagesPermission(ctx context.Context, userID, convID string) error {
	// 这里应该实现具体的权限验证逻辑
	// 例如：验证用户是否有权限访问该会话
	// 简化实现，实际应该根据会话类型和成员关系进行验证
	return nil
}

// sendNotification 发送消息通知
func (uc *MessageUseCase) sendNotification(ctx context.Context, message *entities.Message) error {
	notification := &ports.Notification{
		Type: "new_message",
		Data: map[string]interface{}{
			"message": message.ToDTO(),
		},
	}

	if message.ConvType().IsC2C() {
		// 单聊：发送给接收者
		return uc.notificationSvc.SendToUser(ctx, message.ToUserID(), notification)
	} else if message.ConvType().IsGroup() {
		// 群聊：发送给群组（排除发送者）
		return uc.notificationSvc.SendToGroup(ctx, message.GroupID(), notification)
	}

	return nil
}

// sendRecallNotification 发送撤回通知
func (uc *MessageUseCase) sendRecallNotification(ctx context.Context, message *entities.Message) error {
	notification := &ports.Notification{
		Type: "message_recalled",
		Data: map[string]interface{}{
			"convId":      message.ConvID(),
			"seq":         message.Seq(),
			"serverMsgId": message.ServerMsgID(),
		},
	}

	if message.ConvType().IsC2C() {
		return uc.notificationSvc.SendToUser(ctx, message.ToUserID(), notification)
	} else if message.ConvType().IsGroup() {
		return uc.notificationSvc.SendToGroup(ctx, message.GroupID(), notification)
	}

	return nil
}
