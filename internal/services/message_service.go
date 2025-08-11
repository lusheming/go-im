// Package services 实现业务服务：消息入库与分发、流式消息、撤回/删除、批量关系刷新等。
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"go-im/internal/cache"
	"go-im/internal/models"
	"go-im/internal/mq"
	"go-im/internal/store"

	"github.com/google/uuid"
)

// MessageService 负责消息生命周期：
// - Send：校验/入库/更新会话索引/Redis 下发/可选 Kafka 通知
// - Stream：支持 start/chunk/end 的流式消息持久化与下发
// - Recall/Delete：消息撤回、按用户的会话删除水位
// - List：按会话 seq 游标增量拉取历史
// - DeleteExpired：清理到期的定时自毁消息
// 依赖：MessageStoreInterface + ConvStore + GroupStore（可选 KafkaProducer）
type MessageService struct {
	Store      store.MessageStoreInterface // 使用接口支持多种存储
	ConvStore  *store.ConversationStore
	GroupStore *store.GroupStore
	Producer   *mq.KafkaProducer // 可选

	GroupBatchSize  int
	GroupBatchSleep time.Duration
}

func NewMessageService(ms store.MessageStoreInterface) *MessageService {
	return &MessageService{Store: ms}
}

// SendRequest 发送请求载荷（服务内部），来自 WS/HTTP 层组装。
// - ClientID 作为幂等键，与 convId 构成唯一约束
// - 支持流式消息（IsStreaming/Stream*），以及自毁策略（ExpireAt/BurnAfterRead）
type SendRequest struct {
	ConvID   string                  `json:"convId"`
	ConvType models.ConversationType `json:"convType"`
	ClientID string                  `json:"clientMsgId"`
	From     string                  `json:"from"`
	To       string                  `json:"to,omitempty"`
	GroupID  string                  `json:"groupId,omitempty"`
	Type     string                  `json:"type"`
	Payload  json.RawMessage         `json:"payload"`
	// 流式消息字段
	StreamID     string `json:"streamId,omitempty"`     // 流式消息ID
	StreamSeq    int    `json:"streamSeq,omitempty"`    // 流内序号
	StreamStatus string `json:"streamStatus,omitempty"` // start/chunk/end/error
	IsStreaming  bool   `json:"isStreaming,omitempty"`  // 是否流式
	// 自毁/过期
	ExpireAt      *time.Time `json:"expireAt,omitempty"`      // 定时自毁时间（毫秒）
	BurnAfterRead bool       `json:"burnAfterRead,omitempty"` // 阅后即焚
}

// Deliver 下发给客户端的消息模型（通过 Redis 发布）。
// - 包含服务端生成的 serverMsgId 与会话内有序 seq
// - 若为流式消息，附带流信息；若为自毁消息，携带相应标记
type Deliver struct {
	ServerMsgID string                  `json:"serverMsgId"`
	ClientMsgID string                  `json:"clientMsgId"`
	ConvID      string                  `json:"convId"`
	ConvType    models.ConversationType `json:"convType"`
	From        string                  `json:"from"`
	To          string                  `json:"to,omitempty"`
	GroupID     string                  `json:"groupId,omitempty"`
	Seq         int64                   `json:"seq"`
	Timestamp   int64                   `json:"timestamp"`
	Type        string                  `json:"type"`
	Payload     json.RawMessage         `json:"payload"`
	// 流式消息字段
	StreamID     string `json:"streamId,omitempty"`
	StreamSeq    int    `json:"streamSeq,omitempty"`
	StreamStatus string `json:"streamStatus,omitempty"`
	IsStreaming  bool   `json:"isStreaming,omitempty"`
	// 自毁/过期
	ExpireAt      *time.Time `json:"expireAt,omitempty"`
	BurnAfterRead bool       `json:"burnAfterRead,omitempty"`
}

func lastSeqCacheKey(convID string) string { return fmt.Sprintf("im:lastseq:%s", convID) }
func readSeqCacheKey(userID, convID string) string {
	return fmt.Sprintf("im:readseq:%s:%s", userID, convID)
}
func streamCacheKey(streamID string) string { return fmt.Sprintf("im:stream:%s", streamID) }

// Send 执行消息入库与分发：
// 1) 构造 models.Message；根据自毁策略带上字段
// 2) 入库（流式仅在 start/end 时入库）；更新会话索引与用户-会话关系
// 3) C2C：向双方个人通道发布；Group：向群通道发布（演示）
// 注意：生产环境建议使用严格递增的会话内序列生成器替代 time.Now().UnixNano()
func (s *MessageService) Send(ctx context.Context, req *SendRequest) (*Deliver, error) {
	serverID := uuid.NewString()
	msg := &models.Message{
		ServerMsgID:   serverID,
		ClientMsgID:   req.ClientID,
		ConvID:        req.ConvID,
		ConvType:      req.ConvType,
		FromUserID:    req.From,
		ToUserID:      req.To,
		GroupID:       req.GroupID,
		Seq:           time.Now().UnixNano(), // 演示用序列（生产应使用有序生成）
		Timestamp:     time.Now(),
		Type:          req.Type,
		Payload:       req.Payload,
		StreamID:      req.StreamID,
		StreamSeq:     req.StreamSeq,
		StreamStatus:  req.StreamStatus,
		IsStreaming:   req.IsStreaming,
		ExpireAt:      req.ExpireAt,
		BurnAfterRead: req.BurnAfterRead,
	}
	log.Printf("Msg.Send begin: convId=%s convType=%s from=%s to=%s group=%s clientMsgId=%s seq=%d", req.ConvID, string(req.ConvType), req.From, req.To, req.GroupID, req.ClientID, msg.Seq)

	// 流式消息：只有 start 和 end 状态才入库，chunk 仅实时分发
	shouldStore := !req.IsStreaming || req.StreamStatus == models.StreamStatusStart || req.StreamStatus == models.StreamStatusEnd || req.StreamStatus == models.StreamStatusError
	if shouldStore {
		if err := s.Store.Append(ctx, msg); err != nil {
			log.Printf("Msg.Append error: convId=%s err=%v", req.ConvID, err)
			return nil, err
		}
		log.Printf("Msg.Append ok: convId=%s seq=%d", req.ConvID, msg.Seq)
	}

	// 更新会话索引与用户-会话关系（仅在 start 或非流式时）
	if s.ConvStore != nil && (!req.IsStreaming || req.StreamStatus == models.StreamStatusStart) {
		convTypeStr := string(req.ConvType)
		_ = s.ConvStore.UpsertConversation(ctx, req.ConvID, convTypeStr, req.To, req.GroupID, msg.Seq)
		cache.Client().Set(ctx, lastSeqCacheKey(req.ConvID), msg.Seq, 10*time.Minute)
		_ = s.ConvStore.UpsertUserConversation(ctx, req.From, req.ConvID, convTypeStr, req.To, req.GroupID)
		if req.ConvType == models.ConversationTypeC2C && req.To != "" {
			_ = s.ConvStore.UpsertUserConversation(ctx, req.To, req.ConvID, convTypeStr, req.From, "")
		}
		if req.ConvType == models.ConversationTypeGroup && req.GroupID != "" {
			// MQ 通知：让独立消费者异步批量写 user_conversations（支持百万订阅）
			if s.Producer != nil {
				payload, _ := json.Marshal(map[string]any{"groupId": req.GroupID, "convId": req.ConvID, "from": req.From, "type": convTypeStr, "ts": time.Now().UnixMilli()})
				s.Producer.Publish(payload, []byte(req.GroupID))
				log.Printf("Msg.ConvIndex publish MQ: group=%s convId=%s", req.GroupID, req.ConvID)
			} else if s.GroupStore != nil {
				// 回退：本进程内分片限速批量 upsert（可配置）
				batch := s.GroupBatchSize
				if batch <= 0 {
					batch = 500
				}
				sleep := s.GroupBatchSleep
				if sleep <= 0 {
					sleep = 50 * time.Millisecond
				}
				go func(gid, convID, convTypeStr, from string) {
					ids, err := s.GroupStore.ListMemberIDs(context.Background(), gid)
					if err != nil {
						log.Printf("Msg.ConvIndex list members error: gid=%s err=%v", gid, err)
						return
					}
					for i := 0; i < len(ids); i += batch {
						end := i + batch
						if end > len(ids) {
							end = len(ids)
						}
						for _, uid := range ids[i:end] {
							_ = s.ConvStore.UpsertUserConversation(context.Background(), uid, convID, convTypeStr, from, gid)
						}
						time.Sleep(sleep)
					}
					log.Printf("Msg.ConvIndex fanout done: gid=%s convId=%s members=%d", gid, convID, len(ids))
				}(req.GroupID, req.ConvID, convTypeStr, req.From)
			}
		}
	}

	if req.ConvType == models.ConversationTypeGroup && s.GroupStore != nil {
		// 群禁言检查：全员禁言或成员禁言
		muted, err := s.GroupStore.IsMuted(ctx, req.GroupID, req.From)
		if err == nil && muted {
			log.Printf("Msg.Send denied by mute: group=%s user=%s", req.GroupID, req.From)
			return nil, fmt.Errorf("group is muted or user muted")
		}
	}

	d := &Deliver{
		ServerMsgID:   msg.ServerMsgID,
		ClientMsgID:   msg.ClientMsgID,
		ConvID:        msg.ConvID,
		ConvType:      msg.ConvType,
		From:          msg.FromUserID,
		To:            msg.ToUserID,
		GroupID:       msg.GroupID,
		Seq:           msg.Seq,
		Timestamp:     msg.Timestamp.UnixMilli(),
		Type:          msg.Type,
		Payload:       json.RawMessage(msg.Payload),
		StreamID:      msg.StreamID,
		StreamSeq:     msg.StreamSeq,
		StreamStatus:  msg.StreamStatus,
		IsStreaming:   msg.IsStreaming,
		ExpireAt:      msg.ExpireAt,
		BurnAfterRead: msg.BurnAfterRead,
	}
	// Redis 简化分发
	payload, _ := json.Marshal(d)
	if req.ConvType == models.ConversationTypeC2C {
		err1 := cache.Client().Publish(ctx, cache.DeliverChannel(req.To), payload).Err()
		err2 := cache.Client().Publish(ctx, cache.DeliverChannel(req.From), payload).Err()
		log.Printf("Msg.Publish c2c: convId=%s to=%s err1=%v from=%s err2=%v", req.ConvID, req.To, err1, req.From, err2)
	} else {
		err := cache.Client().Publish(ctx, cache.DeliverChannel(req.GroupID), payload).Err()
		log.Printf("Msg.Publish group: convId=%s group=%s err=%v", req.ConvID, req.GroupID, err)
	}
	return d, nil
}

// StartStream 启动一条流式消息（分多次向同一条消息流追加增量）。
func (s *MessageService) StartStream(ctx context.Context, req *SendRequest) (*Deliver, error) {
	streamID := uuid.NewString()
	req.StreamID = streamID
	req.StreamSeq = 1
	req.StreamStatus = models.StreamStatusStart
	req.IsStreaming = true

	// 缓存流式消息元信息
	streamInfo := map[string]interface{}{
		"convId":    req.ConvID,
		"convType":  req.ConvType,
		"from":      req.From,
		"to":        req.To,
		"groupId":   req.GroupID,
		"startTime": time.Now().UnixMilli(),
		"seq":       1,
	}
	data, _ := json.Marshal(streamInfo)
	cache.Client().Set(ctx, streamCacheKey(streamID), data, 30*time.Minute)

	return s.Send(ctx, req)
}

// SendStreamChunk 发送流式数据块。
// 1) 获取流信息，递增序号
// 2) 构造 SendRequest 并调用 Send 入库
func (s *MessageService) SendStreamChunk(ctx context.Context, streamID string, delta string, metadata map[string]interface{}) error {
	// 获取流信息
	data, err := cache.Client().Get(ctx, streamCacheKey(streamID)).Result()
	if err != nil {
		return fmt.Errorf("stream not found: %s", streamID)
	}

	var streamInfo map[string]interface{}
	if err := json.Unmarshal([]byte(data), &streamInfo); err != nil {
		return err
	}

	// 递增序号
	seq := int(streamInfo["seq"].(float64)) + 1
	streamInfo["seq"] = seq
	updatedData, _ := json.Marshal(streamInfo)
	cache.Client().Set(ctx, streamCacheKey(streamID), updatedData, 30*time.Minute)

	// 构造流式载荷
	payload := models.StreamPayload{Delta: delta, Metadata: metadata}
	payloadBytes, _ := json.Marshal(payload)

	req := &SendRequest{
		ConvID:       streamInfo["convId"].(string),
		ConvType:     models.ConversationType(streamInfo["convType"].(string)),
		ClientID:     streamID + "-" + fmt.Sprintf("%d", seq),
		From:         streamInfo["from"].(string),
		Type:         "stream",
		Payload:      payloadBytes,
		StreamID:     streamID,
		StreamSeq:    seq,
		StreamStatus: models.StreamStatusChunk,
		IsStreaming:  true,
	}
	if to, ok := streamInfo["to"].(string); ok && to != "" {
		req.To = to
	}
	if groupId, ok := streamInfo["groupId"].(string); ok && groupId != "" {
		req.GroupID = groupId
	}

	_, err = s.Send(ctx, req)
	return err
}

// EndStream 结束一条流式消息：收尾并入库最终结果/错误，并清理缓存。
func (s *MessageService) EndStream(ctx context.Context, streamID string, finalText string, errorMsg string) error {
	// 获取流信息
	data, err := cache.Client().Get(ctx, streamCacheKey(streamID)).Result()
	if err != nil {
		return fmt.Errorf("stream not found: %s", streamID)
	}

	var streamInfo map[string]interface{}
	if err := json.Unmarshal([]byte(data), &streamInfo); err != nil {
		return err
	}

	// 构造结束载荷
	payload := models.StreamPayload{Text: finalText, Error: errorMsg}
	payloadBytes, _ := json.Marshal(payload)

	status := models.StreamStatusEnd
	if errorMsg != "" {
		status = models.StreamStatusError
	}

	req := &SendRequest{
		ConvID:       streamInfo["convId"].(string),
		ConvType:     models.ConversationType(streamInfo["convType"].(string)),
		ClientID:     streamID + "-end",
		From:         streamInfo["from"].(string),
		Type:         "stream",
		Payload:      payloadBytes,
		StreamID:     streamID,
		StreamSeq:    int(streamInfo["seq"].(float64)) + 1,
		StreamStatus: status,
		IsStreaming:  true,
	}
	if to, ok := streamInfo["to"].(string); ok && to != "" {
		req.To = to
	}
	if groupId, ok := streamInfo["groupId"].(string); ok && groupId != "" {
		req.GroupID = groupId
	}

	_, err = s.Send(ctx, req)
	// 清理流信息
	cache.Client().Del(ctx, streamCacheKey(streamID))
	return err
}

// Recall 撤回指定消息（按 convId + serverMsgId）。
func (s *MessageService) Recall(ctx context.Context, convID, serverMsgID string) error {
	return s.Store.Recall(ctx, convID, serverMsgID)
}

// DeleteConversation 为用户设置会话删除水位（不物理删历史）。
func (s *MessageService) DeleteConversation(ctx context.Context, ownerID, convID string) error {
	return s.Store.DeleteConversation(ctx, ownerID, convID)
}

// List 按 seq 游标拉取历史。
func (s *MessageService) List(ctx context.Context, convID string, fromSeq int64, limit int) ([]*models.Message, error) {
	return s.Store.List(ctx, convID, fromSeq, limit)
}

// DeleteExpired 清理到期的定时自毁消息（SQL 侧通过定时任务调用；Mongo 侧可由 TTL 索引自动清理）。
func (s *MessageService) DeleteExpired(ctx context.Context, now time.Time) error {
	type expirer interface {
		DeleteExpired(ctx context.Context, before time.Time) error
	}
	if se, ok := s.Store.(expirer); ok {
		return se.DeleteExpired(ctx, now)
	}
	return nil
}

// BurnOnRead 在已读事件后进行“阅后即焚”处理（演示占位，实际逻辑在 WS 层按 seq 撤回）。
func (s *MessageService) BurnOnRead(ctx context.Context, convID string, seq int64, reader string) {
	// 简化：客户端上报 seq，服务端可根据 seq->serverMsgId 映射实现精准撤回
	// 这里作为演示，广播一个 read_burn 事件由客户端清除；或在 Store 层扩展 RecallBySeq
}
