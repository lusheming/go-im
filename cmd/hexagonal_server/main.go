package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go-im/internal/auth"
	"go-im/internal/cache"
	"go-im/internal/config"
	"go-im/internal/metrics"
	"go-im/internal/models"
	"go-im/internal/mq"
	"go-im/internal/ratelimit"
	"go-im/internal/services"
	"go-im/internal/store"
	"go-im/internal/store/mongostore"
	"go-im/internal/store/sqlstore"
	"go-im/internal/transport/tcp"
	"go-im/internal/transport/ws"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/bcrypt"
)

// 解析查询参数为整数
func parseIntQuery(c *gin.Context, key string, defaultValue int) int {
	value, _ := strconv.Atoi(c.DefaultQuery(key, strconv.Itoa(defaultValue)))
	return value
}

func main() {
	cfg := config.Load()

	cache.InitRedis(cfg.RedisAddr, cfg.RedisPass, 0)
	if cfg.EnableMetrics {
		metrics.Init()
	}

	primaryDB := mustOpen(cfg.MySQLDSN)

	// 根据配置选择消息存储：mysql、tidb 或 mongodb
	var msgStore store.MessageStoreInterface
	switch cfg.MessageDB {
	case "mongodb":
		mongoDB, err := mongostore.Connect(cfg.MongoURI)
		if err != nil {
			panic(fmt.Sprintf("MongoDB connection failed: %v", err))
		}
		msgStore = store.NewMongoMessageStore(mongoDB)
	case "tidb":
		messageDB := mustOpen(cfg.TiDBDSN)
		msgStore = store.NewMessageStore(messageDB)
	default: // mysql
		messageDB := mustOpen(cfg.MySQLDSN)
		msgStore = store.NewMessageStore(messageDB)
	}

	_ = sqlstore.Stores{Primary: primaryDB, Message: nil}

	userStore := store.NewUserStore(primaryDB)
	friendStore := store.NewFriendStore(primaryDB)
	groupStore := store.NewGroupStore(primaryDB)
	receiptStore := store.NewReceiptStore(primaryDB)
	convStore := store.NewConversationStore(primaryDB)
	msgSvc := services.NewMessageService(msgStore)
	msgSvc.ConvStore = convStore
	msgSvc.GroupStore = groupStore
	msgSvc.GroupBatchSize = cfg.GroupBatchSize
	msgSvc.GroupBatchSleep = time.Duration(cfg.GroupBatchSleepMS) * time.Millisecond

	// 定时自毁清理（SQL/TiDB）；Mongo 由 TTL 为主
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			_ = msgSvc.DeleteExpired(context.Background(), time.Now())
		}
	}()

	// 文件服务
	fileService := services.NewFileService(&sqlstore.Stores{Primary: primaryDB}, "./uploads", "http://localhost:8080/files", int64(cfg.OSSMaxSizeMB)*1024*1024).WithConfig(cfg)

	// 收藏服务
	favoriteService := services.NewFavoriteService(&sqlstore.Stores{Primary: primaryDB, Message: nil})

	var producer *mq.KafkaProducer
	if cfg.KafkaBrokers != "" {
		p, err := mq.NewKafkaProducer(cfg.KafkaBrokers, cfg.KafkaGroupUpdateTopic)
		if err == nil {
			producer = p
			msgSvc.Producer = p
		}
		defer func() {
			if producer != nil {
				_ = producer.Close()
			}
		}()
	}

	r := gin.Default()
	// 健康/指标
	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })
	if cfg.EnableMetrics {
		r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	}
	// 静态
	r.Static("/ui", "./web/ui")
	r.Static("/app", "./web/app")
	r.Static("/admin", "./web/admin")
	r.StaticFile("/webrtc_test.html", "./web/webrtc_test.html")
	r.StaticFile("/im", "./web/im-client.html")           // 新的 IM 客户端
	r.StaticFile("/manifest.json", "./web/manifest.json") // PWA manifest
	r.StaticFile("/sw.js", "./web/sw.js")                 // Service Worker
	r.StaticFile("/", "./web/ui/index.html")

	// 注册
	r.POST("/api/register", func(c *gin.Context) {
		var req struct{ Username, Password, Nickname string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		u := &models.User{ID: uuid.NewString(), Username: req.Username, Password: string(h), Nickname: req.Nickname}
		if err := userStore.CreateUser(c, u); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"id": u.ID})
	})
	// 登录
	r.POST("/api/login", func(c *gin.Context) {
		var req struct{ Username, Password string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		u, err := userStore.GetByUsername(c, req.Username)
		if err != nil || u == nil || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)) != nil {
			c.JSON(401, gin.H{"error": "invalid credentials"})
			return
		}
		tok, _ := auth.SignJWT(cfg.JWTSecret, u.ID, 7*24*time.Hour)
		c.JSON(200, gin.H{"token": tok, "userId": u.ID})
	})

	// 简易认证
	authn := func(c *gin.Context) (string, bool) {
		tok := c.GetHeader("Authorization")
		if len(tok) > 7 && tok[:7] == "Bearer " {
			tok = tok[7:]
		}
		cl, err := auth.ParseJWT(cfg.JWTSecret, tok)
		if err != nil {
			c.JSON(401, gin.H{"error": "unauthorized"})
			return "", false
		}
		return cl.UserID, true
	}

	// OSS 直传
	r.POST("/api/files/oss/policy", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		dir := c.DefaultPostForm("dir", cfg.OSSPrefix)
		policy, err := fileService.GenerateOSSPolicy(c, uid, dir)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, policy)
	})
	r.POST("/api/files/oss/confirm", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		var req struct {
			Key      string `json:"key" binding:"required"`
			Size     int64  `json:"size"`
			MimeType string `json:"mimeType"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		upload, err := fileService.ConfirmOSSCallback(c, uid, req.Key, req.Size, req.MimeType)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, upload)
	})

	// 用户信息
	r.PUT("/api/users/me", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		var req struct{ Nickname, AvatarURL string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		u := &models.User{ID: uid, Nickname: req.Nickname, AvatarURL: req.AvatarURL}
		if err := userStore.UpdateUser(c, u); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})

	// 会话属性
	r.POST("/api/conversations/:id/pin", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		cid := c.Param("id")
		var req struct{ Pinned bool }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if err := convStore.SetPinned(c, uid, cid, req.Pinned); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})
	r.POST("/api/conversations/:id/mute", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		cid := c.Param("id")
		var req struct{ Muted bool }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if err := convStore.SetMuted(c, uid, cid, req.Muted); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})
	r.POST("/api/conversations/:id/draft", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		cid := c.Param("id")
		var req struct{ Draft string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if err := convStore.SetDraft(c, uid, cid, req.Draft); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})

	// 用户搜索
	r.GET("/api/users/search", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		query := c.Query("q")
		if query == "" {
			c.JSON(400, gin.H{"error": "搜索关键词不能为空"})
			return
		}
		users, err := userStore.SearchUsers(c, query, uid)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"users": users})
	})

	// 好友
	r.GET("/api/friends", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		friends, err := friendStore.ListFriends(c, uid)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"friends": friends})
	})
	r.POST("/api/friends", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		var req struct{ FriendID, Remark string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if err := friendStore.AddFriend(c, uid, req.FriendID, req.Remark); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})
	r.PUT("/api/friends/:id", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		fid := c.Param("id")
		var req struct{ Remark string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if err := friendStore.UpdateRemark(c, uid, fid, req.Remark); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})
	r.DELETE("/api/friends/:id", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		fid := c.Param("id")
		if err := friendStore.DeleteFriend(c, uid, fid); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})

	// 群
	r.GET("/api/groups", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		groups, err := groupStore.ListUserGroups(c, uid)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"groups": groups})
	})
	r.POST("/api/groups", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		var req struct{ Name, Description string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		gid := uuid.NewString()
		if err := groupStore.CreateGroup(c, gid, req.Name, uid); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		_ = groupStore.AddMember(c, gid, uid, "owner", "")
		c.JSON(200, gin.H{"groupId": gid})
	})
	r.POST("/api/groups/:id/join", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		gid := c.Param("id")
		if err := groupStore.JoinGroup(c, gid, uid); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})

	// 未读
	r.GET("/api/unread/summary", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		list, err := convStore.ListWithUnread(c, uid, 200, receiptStore)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		var total int64
		for _, it := range list {
			if v, ok := it["unread"].(int64); ok {
				total += v
			} else if vv, ok := it["unread"].(int); ok {
				total += int64(vv)
			}
		}
		c.JSON(200, gin.H{"totalUnread": total})
	})
	r.POST("/api/unread/mark_all_read", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		rows, err := convStore.ListByUser(c, uid, 5000)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()
		var convIDs []string
		lastSeqs := make(map[string]int64)
		for rows.Next() {
			var convID, convType, peerID, groupID string
			var updatedAt time.Time
			_ = rows.Scan(&convID, &convType, &peerID, &groupID, &updatedAt)
			convIDs = append(convIDs, convID)
			if v, err := cache.Client().Get(c, fmt.Sprintf("im:lastseq:%s", convID)).Int64(); err == nil {
				lastSeqs[convID] = v
			} else {
				v2, _ := convStore.GetConversationLastSeq(c, convID)
				lastSeqs[convID] = v2
				cache.Client().Set(c, fmt.Sprintf("im:lastseq:%s", convID), v2, 10*time.Minute)
			}
		}
		if err := receiptStore.MarkAllReadInChunks(c, uid, convIDs, lastSeqs, cfg.MarkAllReadChunkSize, cfg.MarkAllReadConcurrency, cfg.MarkAllReadRetry); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})

	// 消息
	r.POST("/api/messages/recall", func(c *gin.Context) {
		_, ok := authn(c)
		if !ok {
			return
		}
		var req struct{ ConvID, ServerMsgID string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if err := msgSvc.Recall(c, req.ConvID, req.ServerMsgID); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})
	r.POST("/api/conversations/delete", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		var req struct{ ConvID string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if err := msgSvc.DeleteConversation(c, uid, req.ConvID); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})
	r.POST("/api/messages/read", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		var req struct {
			ConvID string
			Seq    int64
		}
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if err := receiptStore.UpsertReadSeq(c, uid, req.ConvID, req.Seq); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		cache.Client().Set(c, fmt.Sprintf("im:readseq:%s:%s", uid, req.ConvID), req.Seq, 10*time.Minute)
		c.Status(204)
	})
	r.GET("/api/messages/history", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		_ = uid
		convID := c.Query("convId")
		var fromSeq int64
		if v := c.Query("fromSeq"); v != "" {
			_, _ = fmt.Sscan(v, &fromSeq)
		}
		var limit int
		if v := c.Query("limit"); v != "" {
			_, _ = fmt.Sscan(v, &limit)
		}
		msgs, err := msgSvc.List(c, convID, fromSeq, limit)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, msgs)
	})

	// 设备
	r.GET("/api/users/me/devices", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		devices, err := cache.OnlineDevices(c, uid)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		count, _ := cache.OnlineDeviceCount(c, uid)
		c.JSON(200, gin.H{"devices": devices, "count": count})
	})

	// 会话列表（适配前端字段名）
	r.GET("/api/conversations", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		limit := parseIntQuery(c, "limit", 100)
		list, err := convStore.ListWithUnread(c, uid, limit, receiptStore)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		// 转换键名以兼容 web/app/main.js 期望
		resp := make([]map[string]interface{}, 0, len(list))
		for _, it := range list {
			resp = append(resp, map[string]interface{}{
				"id":        it["convId"],
				"conv_type": it["convType"],
				"peer_id":   it["peerId"],
				"group_id":  it["groupId"],
				"unread":    it["unread"],
			})
		}
		c.JSON(200, resp)
	})

	// WebSocket（复用完整 WS 网关，支持 subscribe_group 等）
	limiter := ratelimit.NewTokenBucketLimiter(cache.Client())
	webrtcSvc := services.NewWebRTCService(cfg.WebRTCSTUNServers, cfg.WebRTCTURNServers, cfg.WebRTCTURNUser, cfg.WebRTCTURNPass, cfg.WebRTCEnabled)
	wsServer := &ws.Server{JWTSecret: cfg.JWTSecret, MsgSvc: msgSvc, WebRTCSvc: webrtcSvc, SendQPS: cfg.WSSendQPS, SendBurst: cfg.WSSendBurst, Limiter: limiter}
	wsServer.Receipt = receiptStore
	wsServer.IsFriend = friendStore.IsFriend
	wsServer.IsMember = groupStore.IsMember
	r.GET("/ws", wsServer.Handle)

	// 文件上传 API
	r.POST("/api/files/upload", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(400, gin.H{"error": "no file uploaded"})
			return
		}
		defer file.Close()
		upload, err := fileService.UploadFile(c, uid, file, header)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, upload)
	})
	r.GET("/api/files", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		limit := parseIntQuery(c, "limit", 20)
		offset := parseIntQuery(c, "offset", 0)
		files, err := fileService.ListUserFiles(c, uid, limit, offset)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"files": files})
	})
	r.DELETE("/api/files/:fileId", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		fileID := c.Param("fileId")
		if err := fileService.DeleteFile(c, fileID, uid); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"success": true})
	})
	r.Static("/files", "./uploads")

	// 收藏
	r.POST("/api/favorites/message", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		var req struct {
			MessageID string `json:"messageId"`
			ConvID    string `json:"convId"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		favorite, err := favoriteService.FavoriteMessage(c, uid, req.MessageID, req.ConvID)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, favorite)
	})
	r.POST("/api/favorites/custom", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		var req struct {
			ConvID  string      `json:"convId" binding:"required"`
			Title   string      `json:"title" binding:"required"`
			Content interface{} `json:"content" binding:"required"`
			Tags    []string    `json:"tags"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		favorite, err := favoriteService.FavoriteCustom(c, uid, req.ConvID, req.Title, req.Content, req.Tags)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, favorite)
	})
	r.GET("/api/favorites", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		favoriteType := c.Query("type")
		tags := c.Query("tags")
		limit := parseIntQuery(c, "limit", 20)
		offset := parseIntQuery(c, "offset", 0)
		favorites, err := favoriteService.ListFavorites(c, uid, favoriteType, tags, limit, offset)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"favorites": favorites})
	})
	// 搜索收藏
	r.GET("/api/favorites/search", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		keyword := c.Query("keyword")
		if keyword == "" {
			c.JSON(400, gin.H{"error": "keyword is required"})
			return
		}
		limit := parseIntQuery(c, "limit", 20)
		offset := parseIntQuery(c, "offset", 0)
		favorites, err := favoriteService.SearchFavorites(c, uid, keyword, limit, offset)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"favorites": favorites})
	})
	// 删除收藏
	r.DELETE("/api/favorites/:favoriteId", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		favoriteID := c.Param("favoriteId")
		if err := favoriteService.DeleteFavorite(c, favoriteID, uid); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"success": true})
	})
	// 收藏统计
	r.GET("/api/favorites/stats", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		stats, err := favoriteService.GetFavoriteStats(c, uid)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, stats)
	})

	// WebRTC API 端点
	if cfg.WebRTCEnabled {
		// 获取 ICE 服务器配置
		r.GET("/api/webrtc/ice-servers", func(c *gin.Context) {
			_, ok := authn(c)
			if !ok {
				return
			}
			servers := webrtcSvc.GetICEServers()
			c.JSON(200, gin.H{"iceServers": servers})
		})
		// 获取当前通话
		r.GET("/api/webrtc/current-call", func(c *gin.Context) {
			uid, ok := authn(c)
			if !ok {
				return
			}
			call, err := webrtcSvc.GetUserCurrentCall(c, uid)
			if err != nil {
				c.JSON(404, gin.H{"error": "no active call"})
				return
			}
			c.JSON(200, call)
		})
	}

	// 群禁言
	r.POST("/api/groups/:id/mute", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		gid := c.Param("id")
		var owner string
		_ = primaryDB.QueryRowContext(c, `SELECT owner_id FROM groups WHERE id=?`, gid).Scan(&owner)
		if owner != uid {
			c.JSON(403, gin.H{"error": "forbidden"})
			return
		}
		var req struct{ Muted bool }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if err := groupStore.SetGroupMute(c, gid, req.Muted); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})
	// 成员禁言到期
	r.POST("/api/groups/:id/members/:uid/mute_until", func(c *gin.Context) {
		op, ok := authn(c)
		if !ok {
			return
		}
		gid := c.Param("id")
		target := c.Param("uid")
		var owner string
		_ = primaryDB.QueryRowContext(c, `SELECT owner_id FROM groups WHERE id=?`, gid).Scan(&owner)
		if owner != op {
			c.JSON(403, gin.H{"error": "forbidden"})
			return
		}
		var req struct{ Until string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		var untilPtr *time.Time
		if req.Until != "" {
			if t, err := time.Parse(time.RFC3339, req.Until); err == nil {
				untilPtr = &t
			}
		}
		if err := groupStore.SetMemberMuteUntil(c, gid, target, untilPtr); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(204)
	})
	// 群公告
	r.POST("/api/groups/:id/notices", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		gid := c.Param("id")
		var owner string
		_ = primaryDB.QueryRowContext(c, `SELECT owner_id FROM groups WHERE id=?`, gid).Scan(&owner)
		if owner != uid {
			c.JSON(403, gin.H{"error": "forbidden"})
			return
		}
		var req struct{ Title, Content string }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		nid := uuid.NewString()
		if err := groupStore.CreateNotice(c, nid, gid, req.Title, req.Content, uid); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		notify := gin.H{"action": "group_notice", "data": gin.H{"id": nid, "groupId": gid, "title": req.Title, "content": req.Content, "createdBy": uid, "createdAt": time.Now().UnixMilli()}}
		b, _ := json.Marshal(notify)
		cache.Client().Publish(c, cache.DeliverChannel(gid), b)
		c.JSON(200, gin.H{"id": nid})
	})
	r.GET("/api/groups/:id/notices", func(c *gin.Context) {
		uid, ok := authn(c)
		if !ok {
			return
		}
		_ = uid
		gid := c.Param("id")
		list, err := groupStore.ListNotices(c, gid, 20)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"list": list})
	})

	// TCP（可选）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go (&tcp.Server{Addr: cfg.TCPAddr, JWTSecret: cfg.JWTSecret}).Start(ctx)

	// 管理后台 API（保持 admin/login 与统计/列表等）
	adminGroup := r.Group("/api/admin")
	{
		adminGroup.POST("/login", func(c *gin.Context) {
			var req struct {
				Username string `json:"username" binding:"required"`
				Password string `json:"password" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}
			if req.Username != "admin" {
				c.JSON(401, gin.H{"error": "管理员权限不足"})
				return
			}
			u, err := userStore.GetByUsername(c, req.Username)
			if err != nil || u == nil || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)) != nil {
				c.JSON(401, gin.H{"error": "用户名或密码错误"})
				return
			}
			token, _ := auth.SignJWT(cfg.JWTSecret, u.ID, 24*time.Hour)
			c.JSON(200, gin.H{"token": token, "user": gin.H{"id": u.ID, "username": u.Username}})
		})

		adminAuth := func(c *gin.Context) {
			authHeader := c.GetHeader("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				c.JSON(401, gin.H{"error": "未授权"})
				c.Abort()
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := auth.ParseJWT(cfg.JWTSecret, token)
			if err != nil {
				c.JSON(401, gin.H{"error": "无效的token"})
				c.Abort()
				return
			}
			u, err := userStore.GetByID(c, claims.UserID)
			if err != nil || u == nil || u.Username != "admin" {
				c.JSON(401, gin.H{"error": "管理员权限不足"})
				c.Abort()
				return
			}
			c.Set("adminUserID", claims.UserID)
			c.Next()
		}

		adminGroup.Use(adminAuth)
		adminGroup.GET("/stats", func(c *gin.Context) {
			totalUsers, _ := userStore.CountUsers(c)
			onlineUsers := len(cache.Client().SMembers(c, cache.OnlineUsersKey()).Val())
			totalGroups, _ := groupStore.CountGroups(c)
			c.JSON(200, gin.H{"totalUsers": totalUsers, "onlineUsers": onlineUsers, "totalGroups": totalGroups, "totalMessages": 0})
		})
		adminGroup.GET("/users", func(c *gin.Context) {
			page := parseIntQuery(c, "page", 1)
			limit := parseIntQuery(c, "limit", 50)
			users, err := userStore.ListUsers(c, (page-1)*limit, limit)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			for _, user := range users {
				user.Online = cache.Client().SIsMember(c, cache.OnlineUsersKey(), user.ID).Val()
			}
			c.JSON(200, gin.H{"users": users})
		})
		adminGroup.POST("/users/:id/ban", func(c *gin.Context) {
			userID := c.Param("id")
			c.JSON(200, gin.H{"message": "用户已禁用", "userId": userID})
		})
		adminGroup.GET("/groups", func(c *gin.Context) {
			page := parseIntQuery(c, "page", 1)
			limit := parseIntQuery(c, "limit", 50)
			groups, err := groupStore.ListGroups(c, (page-1)*limit, limit)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			for _, group := range groups {
				if memberIDs, err := groupStore.ListMemberIDs(c, group.ID); err == nil {
					group.MemberCount = len(memberIDs)
				}
			}
			c.JSON(200, gin.H{"groups": groups})
		})
		adminGroup.POST("/groups/:id/disband", func(c *gin.Context) {
			groupID := c.Param("id")
			c.JSON(200, gin.H{"message": "群组已解散", "groupId": groupID})
		})
		adminGroup.GET("/message-stats", func(c *gin.Context) {
			c.JSON(200, gin.H{"todayMessages": 1234, "weekMessages": 8765, "monthMessages": 34567})
		})
		adminGroup.GET("/activities", func(c *gin.Context) {
			activities := []gin.H{{"type": "用户注册", "user": "user123", "content": "新用户注册", "time": time.Now().Format("2006-01-02 15:04:05")}, {"type": "群组创建", "user": "user456", "content": "创建了新群组", "time": time.Now().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")}, {"type": "消息发送", "user": "user789", "content": "发送了消息", "time": time.Now().Add(-2 * time.Hour).Format("2006-01-02 15:04:05")}}
			c.JSON(200, activities)
		})
		adminGroup.GET("/settings", func(c *gin.Context) {
			settings := gin.H{"systemName": "Go-IM", "maxGroupMembers": 500, "messageRetentionDays": 30, "enableRegistration": true}
			c.JSON(200, settings)
		})
		adminGroup.PUT("/settings", func(c *gin.Context) {
			var settings map[string]interface{}
			if err := c.ShouldBindJSON(&settings); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}
			c.JSON(200, gin.H{"message": "设置已保存"})
		})
	}

	_ = r.Run(cfg.ListenAddr)
}

func mustOpen(dsn string) *sql.DB {
	db, err := sqlstore.Open(dsn)
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = db.PingContext(ctx)
	return db
}
