package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go-im/internal/cache"
	"go-im/internal/models"

	"github.com/google/uuid"
)

// WebRTC 服务：负责音视频通话管理与信令
type WebRTCService struct {
	STUNServers []string
	TURNServers []string
	TURNUser    string
	TURNPass    string
	Enabled     bool
}

func NewWebRTCService(stunServers, turnServers []string, turnUser, turnPass string, enabled bool) *WebRTCService {
	return &WebRTCService{
		STUNServers: stunServers,
		TURNServers: turnServers,
		TURNUser:    turnUser,
		TURNPass:    turnPass,
		Enabled:     enabled,
	}
}

// ICE 服务器配置
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// 获取 ICE 服务器配置
func (s *WebRTCService) GetICEServers() []ICEServer {
	var servers []ICEServer

	// 添加 STUN 服务器
	if len(s.STUNServers) > 0 {
		servers = append(servers, ICEServer{URLs: s.STUNServers})
	}

	// 添加 TURN 服务器
	if len(s.TURNServers) > 0 {
		servers = append(servers, ICEServer{
			URLs:       s.TURNServers,
			Username:   s.TURNUser,
			Credential: s.TURNPass,
		})
	}

	return servers
}

// 通话缓存 key
func callCacheKey(callID string) string     { return fmt.Sprintf("im:call:%s", callID) }
func userCallCacheKey(userID string) string { return fmt.Sprintf("im:user:call:%s", userID) }

// 发起通话
func (s *WebRTCService) StartCall(ctx context.Context, fromUserID, toUserID, callType string) (*models.Call, error) {
	if !s.Enabled {
		return nil, fmt.Errorf("WebRTC is disabled")
	}

	// 检查对方是否在通话中
	if existingCallID, err := cache.Client().Get(ctx, userCallCacheKey(toUserID)).Result(); err == nil && existingCallID != "" {
		return nil, fmt.Errorf("user is busy")
	}

	call := &models.Call{
		ID:         uuid.NewString(),
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Type:       callType,
		Status:     models.CallStatusCalling,
		StartTime:  time.Now(),
	}

	// 缓存通话信息
	callData, _ := json.Marshal(call)
	cache.Client().Set(ctx, callCacheKey(call.ID), callData, 30*time.Minute)
	// 用户通话状态设置较短的TTL，防止异常情况下状态不清理
	cache.Client().Set(ctx, userCallCacheKey(fromUserID), call.ID, 5*time.Minute)
	cache.Client().Set(ctx, userCallCacheKey(toUserID), call.ID, 5*time.Minute)

	return call, nil
}

// 接听通话
func (s *WebRTCService) AnswerCall(ctx context.Context, callID, userID string) (*models.Call, error) {
	call, err := s.GetCall(ctx, callID)
	if err != nil {
		return nil, err
	}

	if call.ToUserID != userID {
		return nil, fmt.Errorf("unauthorized to answer this call")
	}

	if call.Status != models.CallStatusCalling && call.Status != models.CallStatusRinging {
		return nil, fmt.Errorf("call cannot be answered in current status: %s", call.Status)
	}

	call.Status = models.CallStatusAnswered

	// 更新缓存
	callData, _ := json.Marshal(call)
	cache.Client().Set(ctx, callCacheKey(call.ID), callData, 30*time.Minute)

	return call, nil
}

// 结束通话
func (s *WebRTCService) EndCall(ctx context.Context, callID, userID string) (*models.Call, error) {
	call, err := s.GetCall(ctx, callID)
	if err != nil {
		return nil, err
	}

	if call.FromUserID != userID && call.ToUserID != userID {
		return nil, fmt.Errorf("unauthorized to end this call")
	}

	now := time.Now()
	call.Status = models.CallStatusEnded
	call.EndTime = &now

	// 计算通话时长
	if call.Status == models.CallStatusAnswered {
		call.Duration = int64(now.Sub(call.StartTime).Seconds())
	}

	// 更新缓存
	callData, _ := json.Marshal(call)
	cache.Client().Set(ctx, callCacheKey(call.ID), callData, 5*time.Minute) // 短期保存

	// 清理用户通话状态
	cache.Client().Del(ctx, userCallCacheKey(call.FromUserID))
	cache.Client().Del(ctx, userCallCacheKey(call.ToUserID))

	return call, nil
}

// 拒接通话
func (s *WebRTCService) RejectCall(ctx context.Context, callID, userID string) (*models.Call, error) {
	call, err := s.GetCall(ctx, callID)
	if err != nil {
		return nil, err
	}

	if call.ToUserID != userID {
		return nil, fmt.Errorf("unauthorized to reject this call")
	}

	call.Status = models.CallStatusRejected
	now := time.Now()
	call.EndTime = &now

	// 更新缓存
	callData, _ := json.Marshal(call)
	cache.Client().Set(ctx, callCacheKey(call.ID), callData, 5*time.Minute)

	// 清理用户通话状态
	cache.Client().Del(ctx, userCallCacheKey(call.FromUserID))
	cache.Client().Del(ctx, userCallCacheKey(call.ToUserID))

	return call, nil
}

// 获取通话信息
func (s *WebRTCService) GetCall(ctx context.Context, callID string) (*models.Call, error) {
	data, err := cache.Client().Get(ctx, callCacheKey(callID)).Result()
	if err != nil {
		return nil, fmt.Errorf("call not found: %s", callID)
	}

	var call models.Call
	if err := json.Unmarshal([]byte(data), &call); err != nil {
		return nil, err
	}

	return &call, nil
}

// 获取用户当前通话
func (s *WebRTCService) GetUserCurrentCall(ctx context.Context, userID string) (*models.Call, error) {
	callID, err := cache.Client().Get(ctx, userCallCacheKey(userID)).Result()
	if err != nil {
		return nil, fmt.Errorf("user has no active call")
	}

	return s.GetCall(ctx, callID)
}

// 转发信令消息
func (s *WebRTCService) ForwardSignaling(ctx context.Context, msg *models.SignalingMessage) error {
	// 验证通话是否存在
	if _, err := s.GetCall(ctx, msg.CallID); err != nil {
		return err
	}

	// 通过 Redis Pub/Sub 转发信令
	deliverMsg := map[string]interface{}{
		"action": "webrtc_signaling",
		"data":   msg,
	}
	deliverData, _ := json.Marshal(deliverMsg)

	// 发送给目标用户
	return cache.Client().Publish(ctx, cache.DeliverChannel(msg.To), deliverData).Err()
}

// 通话超时检查（定期任务）
func (s *WebRTCService) CheckCallTimeouts(ctx context.Context) {
	// 这里可以实现定期检查通话超时的逻辑
	// 例如：超过 60 秒未接听自动挂断
}
