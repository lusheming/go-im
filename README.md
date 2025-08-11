# go-im

一个可集群部署的 Go 语言 IM 最小骨架，支持 WebSocket/TCP 接入，Redis 缓存，MySQL 存结构化数据，TiDB 存消息数据（均可互换）。可选 Kafka 做异步批量分发，内置 Prometheus 指标与限流能力。

## 功能特性
- WebSocket 长连（JWT 认证），TCP 接入（可选）
- 用户/好友/群 基础 CRUD
- 单聊/群聊 消息发送、撤回、历史拉取、会话删除
- 多媒体消息：文本、图片、语音、视频、文件、名片、位置等多种消息类型
- 文件上传管理：支持文件上传、存储、访问控制和过期清理
- 聊天收藏功能：支持收藏消息和自定义内容，提供搜索和标签管理
- 已读回执、会话未读数聚合、未读汇总、标记全已读（分段并发+重试）
- 会话属性：置顶（pinned）/免打扰（muted）/草稿（draft）
- 在线/离线 标记、Redis Pub/Sub 跨节点下发
- 同账号多设备在线：per-device 在线状态管理，消息多设备同步推送
- 流式消息：类似 ChatGPT 的实时输出流（start/chunk/end 状态管理）
- 音视频通话：基于 WebRTC 的实时语音/视频通话，支持 P2P 直连
- 多存储支持：MySQL/TiDB/MongoDB 可选消息存储，灵活适配不同场景
- Kafka（可选）：群会话异步批量更新消费者（支撑百万订阅）
- 速率限制：WS 发送基于 Redis Lua 令牌桶（按用户+设备维度，可配置）
- 可观测性：/metrics 暴露 Prometheus 指标

## 快速开始

1) 依赖准备
- Go 1.22+
- Redis、MySQL（结构化）
- 消息存储（三选一）：MySQL、TiDB、MongoDB
- Kafka（可选，用于群会话异步分发）

2) 初始化数据库
- 结构化（用户/好友/群/回执/会话）：`deployments/sql/schema.mysql.sql`
- 消息存储选择：
  - MySQL：已包含在 `schema.mysql.sql` 中（单库快速启动）
  - TiDB：执行 `deployments/sql/schema.tidb.sql`
  - MongoDB：自动创建集合与索引，无需手动初始化

3) 环境变量（示例）
```bash
export IM_LISTEN_ADDR=":8080"
export IM_TCP_ADDR=":9000"                     # 可选
export IM_REDIS_ADDR="127.0.0.1:6379"
export IM_MYSQL_DSN="root:password@tcp(127.0.0.1:3306)/goim?parseTime=true&loc=Local&charset=utf8mb4"
export IM_TIDB_DSN="root:@tcp(127.0.0.1:4000)/goim?parseTime=true&loc=Local&charset=utf8mb4"
export IM_MONGO_URI="mongodb://127.0.0.1:27017/goim"
export IM_MESSAGE_DB="mysql"                    # mysql|tidb|mongodb（默认 mysql）
export IM_JWT_SECRET="change-me"
# Kafka 可选
export IM_KAFKA_BROKERS="k1:9092,k2:9092"
export IM_KAFKA_GROUP_UPDATE_TOPIC="im-group-update"
# 群会话分片参数（服务端和消费者）
export IM_GROUP_BATCH_SIZE=500
export IM_GROUP_BATCH_SLEEP_MS=50
# 标记全已读分段并发参数
export IM_MARKALLREAD_CHUNK_SIZE=200
export IM_MARKALLREAD_CONCURRENCY=4
export IM_MARKALLREAD_RETRY=3
# WS 发送限流
export IM_WS_SEND_QPS=20
export IM_WS_SEND_BURST=40
# 指标开关
export IM_ENABLE_METRICS=true
# WebRTC 音视频配置
export IM_WEBRTC_ENABLED=true
export IM_WEBRTC_STUN_SERVERS="stun:stun.l.google.com:19302,stun:stun1.l.google.com:19302"
export IM_WEBRTC_TURN_SERVERS=""  # 可选 TURN 服务器
export IM_WEBRTC_TURN_USER=""     # TURN 用户名
export IM_WEBRTC_TURN_PASS=""     # TURN 密码
```

4) 启动
```bash
make tidy && make run            # 主服务（HTTP+WS+/metrics）
go build -o bin/group_consumer ./cmd/group_consumer && ./bin/group_consumer   # 可选：Kafka 消费者
```

## HTTP API（主要）
- 注册：`POST /api/register` {username, password, nickname}
- 登录：`POST /api/login` → {token, userId}
- 更新用户：`PUT /api/users/me` {nickname, avatarUrl}
- 好友：`POST /api/friends`、`PUT /api/friends/:id`、`DELETE /api/friends/:id`
- 群：`POST /api/groups`（建群）、`POST /api/groups/:id/join`（加群）
- 会话属性：
  - 置顶：`POST /api/conversations/:id/pin` {pinned}
  - 免打扰：`POST /api/conversations/:id/mute` {muted}
  - 草稿：`POST /api/conversations/:id/draft` {draft}
- 消息：
  - 撤回：`POST /api/messages/recall` {convId, serverMsgId}
  - 删除会话：`POST /api/conversations/delete` {convId}
  - 已读：`POST /api/messages/read` {convId, seq}
  - 历史：`GET /api/messages/history?convId=...&fromSeq=0&limit=50`
- 会话列表（含属性与未读）：`GET /api/conversations?limit=50`
- 未读汇总：`GET /api/unread/summary` → {totalUnread}
- 标记全已读（分段并发+重试）：`POST /api/unread/mark_all_read`
- 在线设备查询：`GET /api/users/me/devices` → {devices, count}
- WebRTC 音视频：
  - ICE 服务器配置：`GET /api/webrtc/ice-servers` → {iceServers}
  - 当前通话状态：`GET /api/webrtc/current-call` → Call 对象
- 文件管理：
  - 文件上传：`POST /api/files/upload` (multipart/form-data)
  - 文件列表：`GET /api/files?limit=20&offset=0` → {files}
  - 删除文件：`DELETE /api/files/:fileId`
  - 静态文件访问：`GET /files/**` (文件下载)
- 收藏管理：
  - 收藏消息：`POST /api/favorites/message` {messageId, convId}
  - 收藏自定义：`POST /api/favorites/custom` {convId, title, content, tags}
  - 收藏列表：`GET /api/favorites?type=&tags=&limit=20&offset=0` → {favorites}
  - 搜索收藏：`GET /api/favorites/search?keyword=` → {favorites}
  - 删除收藏：`DELETE /api/favorites/:favoriteId`
  - 收藏统计：`GET /api/favorites/stats` → {total, message, custom}
- 健康/指标：`GET /healthz`、`GET /metrics`（需开启）

## WebSocket
- 连接：`GET /ws?token=...&deviceId=...`（或 Header: `Authorization: Bearer <token>`）
- 上行消息（action）：
  - 发送：
    ```json
    {"action":"send","data":{"convId":"c1","convType":"c2c","to":"uidB","type":"text","clientMsgId":"cmid-1","payload":{"text":"hi"}}}
    ```
  - 撤回：`{"action":"recall","data":{"convId":"c1","serverMsgId":"..."}}`
  - 订阅群：`{"action":"subscribe_group","data":{"groupId":"g1"}}`
  - 已读回执：`{"action":"read","data":{"convId":"c1","seq":123}}`
  - 流式消息：
    - 开始：`{"action":"start_stream","data":{"convId":"c1","convType":"c2c","to":"uid","type":"stream","clientMsgId":"s1","payload":{"text":"开始"}}}`
    - 数据块：`{"action":"stream_chunk","data":{"streamId":"xxx","delta":"增量文本"}}`
    - 结束：`{"action":"end_stream","data":{"streamId":"xxx","finalText":"完整文本","error":""}}`
  - WebRTC 音视频通话：
    - 发起通话：`{"action":"call_start","data":{"to":"userId","type":"audio/video"}}`
    - 接听通话：`{"action":"call_answer","data":{"callId":"xxx"}}`
    - 拒接通话：`{"action":"call_reject","data":{"callId":"xxx"}}`
    - 挂断通话：`{"action":"call_end","data":{"callId":"xxx"}}`
    - 信令转发：`{"action":"webrtc_signaling","data":{"callId":"xxx","type":"offer/answer/candidate","sdp":"...","to":"userId"}}`
  - 多媒体消息发送：
    - 文本：`{"action":"send","data":{"convId":"c1","type":"text","payload":{"text":"内容"}}}`
    - 图片：`{"action":"send","data":{"convId":"c1","type":"image","payload":{"url":"...","width":800,"height":600}}}`
    - 语音：`{"action":"send","data":{"convId":"c1","type":"voice","payload":{"url":"...","duration":30}}}`
    - 视频：`{"action":"send","data":{"convId":"c1","type":"video","payload":{"url":"...","duration":120,"thumbnail":"..."}}}`
    - 文件：`{"action":"send","data":{"convId":"c1","type":"file","payload":{"url":"...","name":"doc.pdf","size":1024}}}`
    - 名片：`{"action":"send","data":{"convId":"c1","type":"card","payload":{"userId":"u1","nickname":"张三","avatar":"..."}}}`
    - 位置：`{"action":"send","data":{"convId":"c1","type":"location","payload":{"latitude":39.9,"longitude":116.4,"address":"北京"}}}`
- 注意：WS 发送受限流保护（令牌桶，按用户+设备粒度），超限返回 `{"action":"error","data":{"code":"RATE_LIMIT"}}`；单聊需互为好友、群聊需成员权限。同账号多设备可同时连接，消息会推送至所有在线设备。

## 指标（Prometheus）
- `im_ws_messages_total{action}`：WS 上行动作计数
- `im_send_latency_ms`：消息发送近似耗时（ms）
- 可自行扩展更多业务指标（HTTP 耗时、下行成功、Kafka lag 等）

## 架构说明（要点）
- 存储：
  - MySQL：用户/好友/群/会话/回执；支持消息（单库模式，开发测试便捷）
  - TiDB：消息（含 `UNIQUE(conv_id, client_msg_id)` 幂等）、删除水位（推荐大规模生产）
  - MongoDB：消息/删除水位（文档存储，灵活 schema，适合多媒体消息）
- 消息流：WS → 入库（Append）→ Redis Pub/Sub 下发；群会话可异步经 Kafka 消费者批量更新 `user_conversations`
- 未读：`lastSeq - readSeq`（优先缓存，miss 回源 DB 并回填）
- 标记全已读：按配置分段并发执行事务、失败重试

## MySQL 版 messages 表（便于单库快速启动）
```sql
-- Messages（适配 MySQL，含幂等与检索索引）
CREATE TABLE IF NOT EXISTS messages (
  server_msg_id VARCHAR(64) PRIMARY KEY,
  client_msg_id VARCHAR(128) NOT NULL,
  conv_id VARCHAR(128) NOT NULL,
  conv_type VARCHAR(16) NOT NULL,
  from_user_id VARCHAR(64) NOT NULL,
  to_user_id VARCHAR(64) DEFAULT NULL,
  group_id VARCHAR(64) DEFAULT NULL,
  seq BIGINT NOT NULL,
  timestamp DATETIME NOT NULL,
  type VARCHAR(32) NOT NULL,
  payload LONGBLOB,
  recalled TINYINT(1) NOT NULL DEFAULT 0,
  UNIQUE KEY uniq_conv_client (conv_id, client_msg_id),
  KEY idx_conv_seq (conv_id, seq),
  KEY idx_from_time (from_user_id, timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Per-user conversation delete watermark
CREATE TABLE IF NOT EXISTS conv_deletes (
  owner_id VARCHAR(64) NOT NULL,
  conv_id VARCHAR(128) NOT NULL,
  deleted_at DATETIME NOT NULL,
  PRIMARY KEY(owner_id, conv_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

## 开发/构建
```bash
make build                       # 构建主服务为 bin/im
make run                         # 运行主服务
Go 构建消费者：go build ./cmd/group_consumer
```

## 注意事项
- 注册/登录已使用 bcrypt；生产需增加验证码/风控/审计。
- WS/HTTP 权限、限流、异常重试请按业务加强。
- Kafka 消费者可按分区并发与幂等优化（建议按 groupId/convId 做 key）。

---
如需：K8s 清单、Grafana 看板、压测脚本、Sequencer/Kafka/Pulsar 有序分区方案，请告知，我可补充对应交付。 