package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// TokenBucketLimiter 基于 Redis 的令牌桶限流：
// - 两个键：tokensKey（令牌数）、tsKey（上次补充时间）
// - Lua 原子脚本：计算补充、扣减与过期
// - Allow 出错时可选择“失败即放行”策略（当前实现为放行）
type TokenBucketLimiter struct {
	client *redis.Client
}

func NewTokenBucketLimiter(c *redis.Client) *TokenBucketLimiter {
	return &TokenBucketLimiter{client: c}
}

var luaScript = redis.NewScript(`
local tokens_key = KEYS[1]
local ts_key = KEYS[2]
local rate = tonumber(ARGV[1])        -- 每秒新增令牌
local burst = tonumber(ARGV[2])       -- 桶容量
local now_ms = tonumber(ARGV[3])      -- 当前时间毫秒

local tokens = tonumber(redis.call('GET', tokens_key))
if tokens == nil then tokens = burst end
local ts = tonumber(redis.call('GET', ts_key))
if ts == nil then ts = now_ms end

-- 补充令牌
local delta = math.max(0, now_ms - ts) / 1000.0
local add = delta * rate
local new_tokens = math.min(burst, tokens + add)

local allowed = 0
if new_tokens >= 1 then
  allowed = 1
  new_tokens = new_tokens - 1
end

redis.call('SET', tokens_key, new_tokens)
redis.call('SET', ts_key, now_ms)
redis.call('PEXPIRE', tokens_key, 2000)
redis.call('PEXPIRE', ts_key, 2000)

return {allowed, new_tokens}
`)

// Allow 尝试消耗一个令牌，返回 (allowed, remainingTokens)。
// 参数：
// - key：限流维度（建议 userId:deviceId:action）
// - ratePerSec：每秒生成的令牌数
// - burst：桶容量
func (l *TokenBucketLimiter) Allow(ctx context.Context, key string, ratePerSec, burst int) (bool, int64, error) {
	nowMs := time.Now().UnixMilli()
	vals, err := luaScript.Run(ctx, l.client, []string{key + ":t", key + ":ts"}, ratePerSec, burst, nowMs).Result()
	if err != nil {
		return true, 0, err // 出错时默认放行
	}
	arr := vals.([]interface{})
	allowed := arr[0].(int64) == 1
	rem := int64(0)
	switch v := arr[1].(type) {
	case int64:
		rem = v
	case float64:
		rem = int64(v)
	}
	return allowed, rem, nil
}
