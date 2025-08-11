package config

import (
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddr string `yaml:"listenAddr"`
	TCPAddr    string `yaml:"tcpAddr"`
	RedisAddr  string `yaml:"redisAddr"`
	RedisDB    int    `yaml:"redisDB"`
	RedisPass  string `yaml:"redisPass"`
	MySQLDSN   string `yaml:"mysqlDSN"`
	TiDBDSN    string `yaml:"tidbDSN"`
	MongoURI   string `yaml:"mongoURI"`
	JWTSecret  string `yaml:"jwtSecret"`

	// 消息存储选择：mysql、tidb 或 mongodb（本地默认 mysql，线上建议 tidb/mongodb）
	MessageDB string `yaml:"messageDB"`

	// Kafka 配置（可选）
	KafkaBrokers          string `yaml:"kafkaBrokers"` // 逗号分隔
	KafkaGroupUpdateTopic string `yaml:"kafkaGroupUpdateTopic"`

	// 群成员批量参数
	GroupBatchSize    int `yaml:"groupBatchSize"`
	GroupBatchSleepMS int `yaml:"groupBatchSleepMS"`

	// 标记全已读批量参数
	MarkAllReadChunkSize   int `yaml:"markAllReadChunkSize"`
	MarkAllReadConcurrency int `yaml:"markAllReadConcurrency"`
	MarkAllReadRetry       int `yaml:"markAllReadRetry"`

	// 速率限制（WS 发送）
	WSSendQPS   int `yaml:"wsSendQPS"`
	WSSendBurst int `yaml:"wsSendBurst"`

	// 指标开关
	EnableMetrics bool `yaml:"enableMetrics"`

	// WebRTC 音视频配置
	WebRTCSTUNServers []string `yaml:"webrtcSTUNServers"` // STUN 服务器列表
	WebRTCTURNServers []string `yaml:"webrtcTURNServers"` // TURN 服务器列表
	WebRTCTURNUser    string   `yaml:"webrtcTURNUser"`    // TURN 用户名
	WebRTCTURNPass    string   `yaml:"webrtcTURNPass"`    // TURN 密码
	WebRTCEnabled     bool     `yaml:"webrtcEnabled"`     // 是否启用音视频功能

	// OSS（阿里云）配置
	OSSEnabled         bool   `yaml:"ossEnabled"`
	OSSAccessKeyID     string `yaml:"ossAccessKeyId"`
	OSSAccessKeySecret string `yaml:"ossAccessKeySecret"`
	OSSBucket          string `yaml:"ossBucket"`
	OSSEndpoint        string `yaml:"ossEndpoint"`      // 例如 oss-cn-hangzhou.aliyuncs.com（不含 http）
	OSSPublicHost      string `yaml:"ossPublicHost"`    // 可选：完整域名 https://bucket.oss-cn-hangzhou.aliyuncs.com
	OSSPrefix          string `yaml:"ossPrefix"`        // 目录前缀，如 uploads/
	OSSMaxSizeMB       int    `yaml:"ossMaxSizeMB"`     // 单文件最大 MB
	OSSExpireSeconds   int    `yaml:"ossExpireSeconds"` // policy 过期秒数
}

func Load() *Config {
	// 1) 默认值
	cfg := &Config{
		ListenAddr: ":8080",
		TCPAddr:    "",
		RedisAddr:  "127.0.0.1:6379",
		RedisPass:  "QWEqwe123",
		MySQLDSN:   "root:password@tcp(127.0.0.1:3306)/goim?parseTime=true&loc=Local&charset=utf8mb4",
		TiDBDSN:    "root:@tcp(127.0.0.1:4000)/goim?parseTime=true&loc=Local&charset=utf8mb4",
		MongoURI:   "mongodb://127.0.0.1:27017/goim",
		JWTSecret:  "change-me-in-prod",

		MessageDB: "mysql",

		KafkaBrokers:          "",
		KafkaGroupUpdateTopic: "im-group-update",

		GroupBatchSize:         500,
		GroupBatchSleepMS:      50,
		MarkAllReadChunkSize:   200,
		MarkAllReadConcurrency: 4,
		MarkAllReadRetry:       3,

		WSSendQPS:     20,
		WSSendBurst:   40,
		EnableMetrics: true,

		WebRTCSTUNServers: parseServerList("stun:stun.l.google.com:19302,stun:stun1.l.google.com:19302"),
		WebRTCTURNServers: nil,
		WebRTCTURNUser:    "",
		WebRTCTURNPass:    "",
		WebRTCEnabled:     true,

		OSSEnabled:         false,
		OSSAccessKeyID:     "",
		OSSAccessKeySecret: "",
		OSSBucket:          "",
		OSSEndpoint:        "",
		OSSPublicHost:      "",
		OSSPrefix:          "uploads/",
		OSSMaxSizeMB:       50,
		OSSExpireSeconds:   60,
	}

	// 2) YAML 覆盖（如果有）
	configPath := getEnv("IM_CONFIG_FILE", getEnv("CONFIG_FILE", "config.yml"))
	if st, err := os.Stat(configPath); err == nil && !st.IsDir() {
		if data, err2 := os.ReadFile(configPath); err2 == nil {
			_ = yaml.Unmarshal(data, cfg)
		}
	}

	// 3) 环境变量覆盖 YAML
	applyEnv(cfg)
	return cfg
}

func applyEnv(cfg *Config) {
	setStr := func(env string, dst *string) {
		if v := os.Getenv(env); v != "" {
			*dst = v
		}
	}
	setInt := func(env string, dst *int) {
		if v := os.Getenv(env); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				*dst = n
			}
		}
	}
	setBool := func(env string, dst *bool) {
		if v := os.Getenv(env); v != "" {
			*dst = (v == "true" || v == "1" || v == "yes")
		}
	}
	setList := func(env string, dst *[]string) {
		if v := os.Getenv(env); v != "" {
			*dst = parseServerList(v)
		}
	}

	setStr("IM_LISTEN_ADDR", &cfg.ListenAddr)
	setStr("IM_TCP_ADDR", &cfg.TCPAddr)
	setStr("IM_REDIS_ADDR", &cfg.RedisAddr)
	setStr("IM_REDIS_PASS", &cfg.RedisPass)
	setInt("IM_REDIS_DB", &cfg.RedisDB)
	setStr("IM_MYSQL_DSN", &cfg.MySQLDSN)
	setStr("IM_TIDB_DSN", &cfg.TiDBDSN)
	setStr("IM_MONGO_URI", &cfg.MongoURI)
	setStr("IM_JWT_SECRET", &cfg.JWTSecret)

	setStr("IM_MESSAGE_DB", &cfg.MessageDB)

	setStr("IM_KAFKA_BROKERS", &cfg.KafkaBrokers)
	setStr("IM_KAFKA_GROUP_UPDATE_TOPIC", &cfg.KafkaGroupUpdateTopic)

	setInt("IM_GROUP_BATCH_SIZE", &cfg.GroupBatchSize)
	setInt("IM_GROUP_BATCH_SLEEP_MS", &cfg.GroupBatchSleepMS)
	setInt("IM_MARKALLREAD_CHUNK_SIZE", &cfg.MarkAllReadChunkSize)
	setInt("IM_MARKALLREAD_CONCURRENCY", &cfg.MarkAllReadConcurrency)
	setInt("IM_MARKALLREAD_RETRY", &cfg.MarkAllReadRetry)

	setInt("IM_WS_SEND_QPS", &cfg.WSSendQPS)
	setInt("IM_WS_SEND_BURST", &cfg.WSSendBurst)
	setBool("IM_ENABLE_METRICS", &cfg.EnableMetrics)

	setList("IM_WEBRTC_STUN_SERVERS", &cfg.WebRTCSTUNServers)
	setList("IM_WEBRTC_TURN_SERVERS", &cfg.WebRTCTURNServers)
	setStr("IM_WEBRTC_TURN_USER", &cfg.WebRTCTURNUser)
	setStr("IM_WEBRTC_TURN_PASS", &cfg.WebRTCTURNPass)
	setBool("IM_WEBRTC_ENABLED", &cfg.WebRTCEnabled)

	setBool("IM_OSS_ENABLED", &cfg.OSSEnabled)
	setStr("IM_OSS_ACCESS_KEY_ID", &cfg.OSSAccessKeyID)
	setStr("IM_OSS_ACCESS_KEY_SECRET", &cfg.OSSAccessKeySecret)
	setStr("IM_OSS_BUCKET", &cfg.OSSBucket)
	setStr("IM_OSS_ENDPOINT", &cfg.OSSEndpoint)
	setStr("IM_OSS_PUBLIC_HOST", &cfg.OSSPublicHost)
	setStr("IM_OSS_PREFIX", &cfg.OSSPrefix)
	setInt("IM_OSS_MAX_SIZE_MB", &cfg.OSSMaxSizeMB)
	setInt("IM_OSS_EXPIRE_SECONDS", &cfg.OSSExpireSeconds)
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int { // kept for backwards compatibility
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}

// 解析服务器列表（逗号分隔）
func parseServerList(s string) []string {
	if s == "" {
		return nil
	}
	var servers []string
	for i := 0; i < len(s); {
		start := i
		for i < len(s) && s[i] != ',' {
			i++
		}
		if start < i {
			server := s[start:i]
			// 去除空格
			for len(server) > 0 && server[0] == ' ' {
				server = server[1:]
			}
			for len(server) > 0 && server[len(server)-1] == ' ' {
				server = server[:len(server)-1]
			}
			if server != "" {
				servers = append(servers, server)
			}
		}
		if i < len(s) {
			i++ // skip comma
		}
	}
	return servers
}
