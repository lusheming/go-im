// Package ws 提供 WebSocket 接入网关：处理认证、连接生命周期、上行动作（发送/已读/订阅/信令等）与下行分发（通过 Redis Pub/Sub）。
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"go-im/internal/auth"
	"go-im/internal/cache"
	"go-im/internal/metrics"
	"go-im/internal/models"
	"go-im/internal/ratelimit"
	"go-im/internal/services"
	"go-im/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Server 是 WebSocket 网关服务。
// - 注入消息服务 MsgSvc 以完成消息入库与分发
// - 注入权限回调 IsFriend/IsMember 做发送前的快速业务校验
// - 基于 Redis 令牌桶对上行发送做速率限制，防止滥用
// - 每个连接使用单独的写锁，避免并发写触发 gorilla/websocket 冲突
type Server struct {
	JWTSecret string
	MsgSvc    *services.MessageService
	WebRTCSvc *services.WebRTCService // WebRTC 服务
	Receipt   *store.ReceiptStore     // 已读回执存储
	// 权限回调：用于校验单聊是否好友、群聊是否成员
	IsFriend func(ctx context.Context, a, b string) (bool, error)
	IsMember func(ctx context.Context, gid, uid string) (bool, error)

	// 速率限制参数
	SendQPS   int
	SendBurst int
	Limiter   *ratelimit.TokenBucketLimiter
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSMessage 统一封装上行的动作与数据载荷。
// action 示例：send、recall、read、subscribe_group、start_stream、stream_chunk、end_stream、webrtc_signaling
type WSMessage struct {
	Action string          `json:"action"` // send, recall, read, subscribe_group, start_stream, stream_chunk, end_stream, call_start, call_answer, call_reject, call_end, webrtc_signaling
	Data   json.RawMessage `json:"data"`
}

// SendPayload 客户端发送消息时的载荷。
// - 支持 burnAfterRead（阅后即焚）与 expireAtMs（定时自毁）
// - convId/convType 指明路由维度，c2c 需提供 to，group 需提供 groupId
type SendPayload struct {
	ConvID   string          `json:"convId"`
	ConvType string          `json:"convType"`
	To       string          `json:"to,omitempty"`
	GroupID  string          `json:"groupId,omitempty"`
	Type     string          `json:"type"`
	ClientID string          `json:"clientMsgId"`
	Payload  json.RawMessage `json:"payload"`
	// 自毁/过期
	ExpireAtMS    int64 `json:"expireAtMs,omitempty"`    // 过期时间戳（毫秒）
	BurnAfterRead bool  `json:"burnAfterRead,omitempty"` // 阅后即焚
}

// 撤回负载
type RecallPayload struct {
	ConvID      string `json:"convId"`
	ServerMsgID string `json:"serverMsgId"`
}

// 已读回执负载
type ReadPayload struct {
	ConvID string `json:"convId"`
	Seq    int64  `json:"seq"`
}

// 订阅群聊负载
type SubscribeGroupPayload struct {
	GroupID string `json:"groupId"`
}

// 流式消息负载
type StartStreamPayload struct {
	ConvID   string          `json:"convId"`
	ConvType string          `json:"convType"`
	To       string          `json:"to,omitempty"`
	GroupID  string          `json:"groupId,omitempty"`
	Type     string          `json:"type"`
	ClientID string          `json:"clientMsgId"`
	Payload  json.RawMessage `json:"payload"`
}

type StreamChunkPayload struct {
	StreamID string                 `json:"streamId"`
	Delta    string                 `json:"delta"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type EndStreamPayload struct {
	StreamID  string `json:"streamId"`
	FinalText string `json:"finalText,omitempty"`
	Error     string `json:"error,omitempty"`
}

// 正在输入负载
type TypingPayload struct {
	ConvID   string `json:"convId"`
	ConvType string `json:"convType"`
	To       string `json:"to,omitempty"`
	GroupID  string `json:"groupId,omitempty"`
	Typing   bool   `json:"typing"`
}

// WebRTC 通话负载
type CallStartPayload struct {
	To   string `json:"to"`   // 被叫用户ID
	Type string `json:"type"` // audio/video
}

type CallControlPayload struct {
	CallID string `json:"callId"` // 通话ID
}

type WebRTCSignalingPayload struct {
	CallID    string      `json:"callId"`
	Type      string      `json:"type"` // offer/answer/candidate/hangup
	SDP       string      `json:"sdp,omitempty"`
	Candidate interface{} `json:"candidate,omitempty"`
	To        string      `json:"to"` // 目标用户
}

// Handle 处理 HTTP 升级为 WebSocket，以及该连接的读/写循环。
// - 认证：支持 URL 查询参数或 Authorization: Bearer 传递 JWT
// - 上线/下线：多设备在线集合，连接退出自动下线
// - 下行：订阅个人投递通道，将 Redis 消息写回客户端
func (s *Server) Handle(c *gin.Context) {
	ctx := c.Request.Context()
	token := c.Query("token")
	if token == "" {
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	claims, err := auth.ParseJWT(s.JWTSecret, token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	deviceID := c.Query("deviceId")
	if deviceID == "" {
		deviceID = "web-" + time.Now().Format("150405.000")
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	userID := claims.UserID
	log.Printf("WS connected: user=%s device=%s", userID, deviceID)
	_ = cache.SetDeviceOnline(ctx, userID, deviceID)
	defer func() {
		cache.SetDeviceOffline(context.Background(), userID, deviceID)
		log.Printf("WS disconnected: user=%s device=%s", userID, deviceID)
	}()

	// 每个连接的写锁，序列化所有写操作，避免 concurrent write
	writeMu := &sync.Mutex{}

	// 订阅个人下发通道
	sub := cache.Client().Subscribe(ctx, cache.DeliverChannel(userID))
	defer sub.Close()

	// 读循环：处理客户端上行动作
	go func() {
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WS read error: user=%s err=%v", userID, err)
				return
			}
			if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
				continue
			}
			var m WSMessage
			if err := json.Unmarshal(data, &m); err != nil {
				log.Printf("WS unmarshal error: user=%s err=%v data=%q", userID, err, string(data))
				continue
			}
			metrics.WSMessagesTotal.WithLabelValues(m.Action).Inc()
			log.Printf("WS inbound: user=%s action=%s size=%d", userID, m.Action, len(data))
			s.handleInbound(ctx, userID, deviceID, conn, writeMu, &m)
		}
	}()

	// 写循环：将 Redis 收到的消息发给客户端
	for {
		msg, err := sub.ReceiveMessage(ctx)
		if err != nil {
			log.Printf("WS redis receive error: user=%s err=%v", userID, err)
			return
		}
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		writeMu.Lock()
		err = conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
		writeMu.Unlock()
		if err != nil {
			log.Printf("WS write error: user=%s err=%v", userID, err)
			return
		}
	}
}

// rateLimitAllow 使用 Redis 令牌桶对用户+设备维度的发送做限速。
// - 默认 QPS=20，突发=40，可通过配置调整
// - 出错时当前实现放行（可按需调整策略）
func (s *Server) rateLimitAllow(ctx context.Context, userID, deviceID string) bool {
	qps := s.SendQPS
	burst := s.SendBurst
	if qps <= 0 {
		qps = 20
	}
	if burst <= 0 {
		burst = 40
	}
	if s.Limiter == nil {
		return true
	}
	allowed, _, _ := s.Limiter.Allow(ctx, "im:tb:ws:send:"+userID+":"+deviceID, qps, burst)
	return allowed
}

// handleInbound 处理上行动作，入口统一在这里分发：
// - send：权限校验 → 调用 MsgSvc.Send 入库与分发 → 返回 ack
// - read：写入已读回执 →（若阅后即焚）按 seq 撤回并广播 recalled 事件
// - subscribe_group：订阅群通道（演示模式，生产建议服务端 fan-out 至用户私有通道）
// - 其它：typing、WebRTC 信令等
func (s *Server) handleInbound(ctx context.Context, userID, deviceID string, conn *websocket.Conn, writeMu *sync.Mutex, m *WSMessage) {
	switch m.Action {
	case "send":
		log.Printf("WS handleInbound SEND start: user=%s deviceId=%s", userID, deviceID)
		if !s.rateLimitAllow(ctx, userID, deviceID) {
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","data":{"code":"RATE_LIMIT"}}`))
			writeMu.Unlock()
			log.Printf("WS send blocked by rate limit: user=%s device=%s", userID, deviceID)
			return
		}
		log.Printf("WS rate limit passed: user=%s", userID)
		var p SendPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			log.Printf("WS send payload unmarshal error: user=%s err=%v", userID, err)
			return
		}
		log.Printf("WS payload unmarshaled: user=%s payload=%+v", userID, p)
		convType := services.ToConvType(p.ConvType)
		log.Printf("WS send: user=%s convId=%s convType=%s to=%s group=%s clientMsgId=%s", userID, p.ConvID, p.ConvType, p.To, p.GroupID, p.ClientID)
		// 发送权限校验
		log.Printf("WS permission check: user=%s convType=%s to=%s group=%s", userID, p.ConvType, p.To, p.GroupID)
		if convType == models.ConversationTypeC2C && s.IsFriend != nil {
			log.Printf("WS checking friend relationship: user=%s to=%s", userID, p.To)
			ok, ferr := s.IsFriend(ctx, userID, p.To)
			if ferr != nil {
				log.Printf("WS isFriend error: user=%s to=%s err=%v", userID, p.To, ferr)
			}
			log.Printf("WS isFriend result: user=%s to=%s isFriend=%v err=%v", userID, p.To, ok, ferr)
			if !ok {
				writeMu.Lock()
				conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","data":{"code":"NOT_FRIEND"}}`))
				writeMu.Unlock()
				log.Printf("WS send denied NOT_FRIEND: user=%s to=%s", userID, p.To)
				return
			}
		}
		if convType == models.ConversationTypeGroup && s.IsMember != nil {
			ok, merr := s.IsMember(ctx, p.GroupID, userID)
			if merr != nil {
				log.Printf("WS isMember error: user=%s group=%s err=%v", userID, p.GroupID, merr)
			}
			if !ok {
				writeMu.Lock()
				conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","data":{"code":"NOT_GROUP_MEMBER"}}`))
				writeMu.Unlock()
				log.Printf("WS send denied NOT_GROUP_MEMBER: user=%s group=%s", userID, p.GroupID)
				return
			}
		}
		log.Printf("WS permission check passed: user=%s", userID)
		start := time.Now()
		var expireAtPtr *time.Time
		if p.ExpireAtMS > 0 {
			t := time.UnixMilli(p.ExpireAtMS)
			expireAtPtr = &t
		}
		log.Printf("WS calling MsgSvc.Send: user=%s convId=%s", userID, p.ConvID)
		d, err := s.MsgSvc.Send(ctx, &services.SendRequest{ConvID: p.ConvID, ConvType: convType, ClientID: p.ClientID, From: userID, To: p.To, GroupID: p.GroupID, Type: p.Type, Payload: p.Payload, ExpireAt: expireAtPtr, BurnAfterRead: p.BurnAfterRead})
		log.Printf("WS MsgSvc.Send result: user=%s convId=%s err=%v", userID, p.ConvID, err)
		metrics.MessageSendLatency.Observe(float64(time.Since(start).Milliseconds()))
		if err == nil {
			b, _ := json.Marshal(gin.H{"action": "ack", "data": d})
			writeMu.Lock()
			werr := conn.WriteMessage(websocket.TextMessage, b)
			writeMu.Unlock()
			log.Printf("WS send ack: user=%s convId=%s seq=%d writeErr=%v", userID, p.ConvID, d.Seq, werr)
			// 解析 mentions 并下发提醒
			var body map[string]interface{}
			if json.Unmarshal(p.Payload, &body) == nil {
				if arr, ok := body["mentions"].([]interface{}); ok && convType == models.ConversationTypeGroup {
					for _, v := range arr {
						if uid, ok := v.(string); ok && uid != "" {
							tip := gin.H{"action": "mention", "data": gin.H{"groupId": p.GroupID, "convId": p.ConvID, "from": userID, "seq": d.Seq}}
							tipb, _ := json.Marshal(tip)
							err2 := cache.Client().Publish(ctx, cache.DeliverChannel(uid), tipb).Err()
							if err2 != nil {
								log.Printf("WS mention publish error: to=%s err=%v", uid, err2)
							}
						}
					}
				}
			}
		} else {
			b, _ := json.Marshal(gin.H{"action": "error", "data": gin.H{"code": "SEND_FAILED", "message": err.Error()}})
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, b)
			writeMu.Unlock()
			log.Printf("WS send failed: user=%s convId=%s err=%v", userID, p.ConvID, err)
		}
	case "start_stream":
		if !s.rateLimitAllow(ctx, userID, deviceID) {
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","data":{"code":"RATE_LIMIT"}}`))
			writeMu.Unlock()
			return
		}
		var p StartStreamPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return
		}
		convType := services.ToConvType(p.ConvType)
		// 发送权限校验
		if convType == models.ConversationTypeC2C && s.IsFriend != nil {
			ok, _ := s.IsFriend(ctx, userID, p.To)
			if !ok {
				writeMu.Lock()
				conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","data":{"code":"NOT_FRIEND"}}`))
				writeMu.Unlock()
				return
			}
		}
		if convType == models.ConversationTypeGroup && s.IsMember != nil {
			ok, _ := s.IsMember(ctx, p.GroupID, userID)
			if !ok {
				writeMu.Lock()
				conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","data":{"code":"NOT_GROUP_MEMBER"}}`))
				writeMu.Unlock()
				return
			}
		}
		d, err := s.MsgSvc.StartStream(ctx, &services.SendRequest{ConvID: p.ConvID, ConvType: convType, ClientID: p.ClientID, From: userID, To: p.To, GroupID: p.GroupID, Type: p.Type, Payload: p.Payload})
		if err == nil {
			b, _ := json.Marshal(gin.H{"action": "stream_started", "data": d})
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, b)
			writeMu.Unlock()
		} else {
			b, _ := json.Marshal(gin.H{"action": "error", "data": gin.H{"code": "STREAM_START_FAILED", "message": err.Error()}})
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, b)
			writeMu.Unlock()
		}
	case "stream_chunk":
		var p StreamChunkPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return
		}
		err := s.MsgSvc.SendStreamChunk(ctx, p.StreamID, p.Delta, p.Metadata)
		if err != nil {
			b, _ := json.Marshal(gin.H{"action": "error", "data": gin.H{"code": "STREAM_ERROR", "message": err.Error()}})
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, b)
			writeMu.Unlock()
		}
	case "end_stream":
		var p EndStreamPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return
		}
		err := s.MsgSvc.EndStream(ctx, p.StreamID, p.FinalText, p.Error)
		if err == nil {
			b, _ := json.Marshal(gin.H{"action": "stream_ended", "data": gin.H{"streamId": p.StreamID}})
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, b)
			writeMu.Unlock()
		}
	// WebRTC 通话控制（省略其他分支中相同写操作，均加锁）
	case "call_start":
		if s.WebRTCSvc == nil || !s.WebRTCSvc.Enabled {
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","data":{"code":"WEBRTC_DISABLED"}}`))
			writeMu.Unlock()
			return
		}
		var p CallStartPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return
		}
		if s.IsFriend != nil {
			ok, _ := s.IsFriend(ctx, userID, p.To)
			if !ok {
				writeMu.Lock()
				conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","data":{"code":"NOT_FRIEND"}}`))
				writeMu.Unlock()
				return
			}
		}
		call, err := s.WebRTCSvc.StartCall(ctx, userID, p.To, p.Type)
		if err != nil {
			b, _ := json.Marshal(gin.H{"action": "error", "data": gin.H{"code": "CALL_FAILED", "message": err.Error()}})
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, b)
			writeMu.Unlock()
			return
		}
		b, _ := json.Marshal(gin.H{"action": "call_started", "data": call})
		writeMu.Lock()
		conn.WriteMessage(websocket.TextMessage, b)
		writeMu.Unlock()
		notifyData, _ := json.Marshal(gin.H{"action": "call_incoming", "data": call})
		cache.Client().Publish(ctx, cache.DeliverChannel(p.To), notifyData)
	case "call_answer":
		if s.WebRTCSvc == nil || !s.WebRTCSvc.Enabled {
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","data":{"code":"WEBRTC_DISABLED"}}`))
			writeMu.Unlock()
			return
		}
		var p CallControlPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return
		}
		call, err := s.WebRTCSvc.AnswerCall(ctx, p.CallID, userID)
		if err != nil {
			b, _ := json.Marshal(gin.H{"action": "error", "data": gin.H{"code": "CALL_ANSWER_FAILED", "message": err.Error()}})
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, b)
			writeMu.Unlock()
			return
		}
		answerData, _ := json.Marshal(gin.H{"action": "call_answered", "data": call})
		writeMu.Lock()
		conn.WriteMessage(websocket.TextMessage, answerData)
		writeMu.Unlock()
		cache.Client().Publish(ctx, cache.DeliverChannel(call.FromUserID), answerData)
	case "call_reject":
		if s.WebRTCSvc == nil {
			return
		}
		var p CallControlPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return
		}
		call, err := s.WebRTCSvc.RejectCall(ctx, p.CallID, userID)
		if err == nil {
			rejectData, _ := json.Marshal(gin.H{"action": "call_rejected", "data": call})
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, rejectData)
			writeMu.Unlock()
			cache.Client().Publish(ctx, cache.DeliverChannel(call.FromUserID), rejectData)
		}
	case "call_end":
		if s.WebRTCSvc == nil {
			return
		}
		var p CallControlPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return
		}
		call, err := s.WebRTCSvc.EndCall(ctx, p.CallID, userID)
		if err == nil {
			endData, _ := json.Marshal(gin.H{"action": "call_ended", "data": call})
			writeMu.Lock()
			conn.WriteMessage(websocket.TextMessage, endData)
			writeMu.Unlock()
			otherUserID := call.FromUserID
			if call.FromUserID == userID {
				otherUserID = call.ToUserID
			}
			cache.Client().Publish(ctx, cache.DeliverChannel(otherUserID), endData)
		}
	case "webrtc_signaling":
		if s.WebRTCSvc == nil {
			return
		}
		var p WebRTCSignalingPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return
		}
		sigMsg := &models.SignalingMessage{Type: p.Type, CallID: p.CallID, From: userID, To: p.To, SDP: p.SDP, Candidate: p.Candidate}
		s.WebRTCSvc.ForwardSignaling(ctx, sigMsg)
	case "typing":
		var p TypingPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			log.Printf("WS typing unmarshal error: user=%s err=%v", userID, err)
			return
		}
		convType := services.ToConvType(p.ConvType)
		if convType == models.ConversationTypeC2C && s.IsFriend != nil {
			ok, _ := s.IsFriend(ctx, userID, p.To)
			if !ok {
				log.Printf("WS typing denied NOT_FRIEND: user=%s to=%s", userID, p.To)
				return
			}
		}
		if convType == models.ConversationTypeGroup && s.IsMember != nil {
			ok, _ := s.IsMember(ctx, p.GroupID, userID)
			if !ok {
				log.Printf("WS typing denied NOT_GROUP_MEMBER: user=%s group=%s", userID, p.GroupID)
				return
			}
		}
		notify := gin.H{"action": "typing", "data": gin.H{"convId": p.ConvID, "convType": p.ConvType, "from": userID, "to": p.To, "groupId": p.GroupID, "typing": p.Typing, "ts": time.Now().UnixMilli()}}
		b, _ := json.Marshal(notify)
		if convType == models.ConversationTypeC2C {
			err := cache.Client().Publish(ctx, cache.DeliverChannel(p.To), b).Err()
			if err != nil {
				log.Printf("WS typing publish error: user=%s to=%s err=%v", userID, p.To, err)
			}
		} else if convType == models.ConversationTypeGroup {
			err := cache.Client().Publish(ctx, cache.DeliverChannel(p.GroupID), b).Err()
			if err != nil {
				log.Printf("WS typing publish error: user=%s group=%s err=%v", userID, p.GroupID, err)
			}
		}
	case "recall":
		var p RecallPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return
		}
		_ = s.MsgSvc.Recall(ctx, p.ConvID, p.ServerMsgID)
	case "read":
		var p ReadPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return
		}
		// 1) 本地 ACK
		b, _ := json.Marshal(gin.H{"action": "read_ack", "data": p})
		writeMu.Lock()
		conn.WriteMessage(websocket.TextMessage, b)
		writeMu.Unlock()
		// 1.5) 写入已读回执并更新缓存
		if s.Receipt != nil {
			_ = s.Receipt.UpsertReadSeq(ctx, userID, p.ConvID, p.Seq)
			cache.Client().Set(ctx, fmt.Sprintf("im:readseq:%s:%s", userID, p.ConvID), p.Seq, 10*time.Minute)
		}
		// 2) 如果为阅后即焚，尝试按 seq 撤回，并广播给相关用户
		if s.MsgSvc != nil && s.MsgSvc.Store != nil {
			msg, err := s.MsgSvc.Store.GetBySeq(ctx, p.ConvID, p.Seq)
			if err == nil && msg != nil && msg.BurnAfterRead && !msg.Recalled {
				if err2 := s.MsgSvc.Store.RecallBySeq(ctx, p.ConvID, p.Seq); err2 == nil {
					evt, _ := json.Marshal(gin.H{"action": "recalled", "data": gin.H{"convId": p.ConvID, "seq": p.Seq}})
					if msg.ConvType == models.ConversationTypeC2C {
						if msg.ToUserID != "" {
							_ = cache.Client().Publish(ctx, cache.DeliverChannel(msg.ToUserID), evt).Err()
						}
						if msg.FromUserID != "" {
							_ = cache.Client().Publish(ctx, cache.DeliverChannel(msg.FromUserID), evt).Err()
						}
					} else if msg.ConvType == models.ConversationTypeGroup {
						if msg.GroupID != "" {
							_ = cache.Client().Publish(ctx, cache.DeliverChannel(msg.GroupID), evt).Err()
						}
					}
				}
			}
		}
	case "subscribe_group":
		var p SubscribeGroupPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			log.Printf("WS subscribe_group unmarshal error: user=%s err=%v", userID, err)
			return
		}
		log.Printf("WS subscribe_group: user=%s group=%s", userID, p.GroupID)
		go func(gid string) {
			sub := cache.Client().Subscribe(ctx, cache.DeliverChannel(gid))
			for {
				msg, err := sub.ReceiveMessage(ctx)
				if err != nil {
					log.Printf("WS group sub receive error: user=%s group=%s err=%v", userID, gid, err)
					return
				}
				writeMu.Lock()
				err = conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
				writeMu.Unlock()
				if err != nil {
					log.Printf("WS group sub write error: user=%s group=%s err=%v", userID, gid, err)
					return
				}
			}
		}(p.GroupID)
	}
}
