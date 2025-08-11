package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// 本包封装了 Redis 客户端与常用的在线状态/通道键：
// - 在线集合：im:presence:online
// - 用户设备集合：im:presence:devices:<userId>
// - 投递通道：im:deliver:<userId>
// 提供多设备上线/下线的原子更新，以及便捷的在线查询接口。
var (
	redisClient *redis.Client
)

func InitRedis(addr, pass string, db int) {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pass,
		DB:       db,
	})
}

func Client() *redis.Client { return redisClient }

// PresenceKey 返回用户在线键；OnlineUsersKey 返回全局在线集合键；DeliverChannel 返回用户投递通道。
func PresenceKey(userID string) string       { return fmt.Sprintf("im:presence:%s", userID) }
func OnlineUsersKey() string                 { return "im:presence:online" }
func DeliverChannel(userID string) string    { return fmt.Sprintf("im:deliver:%s", userID) }
func DevicePresenceKey(userID string) string { return fmt.Sprintf("im:presence:devices:%s", userID) }

// 兼容旧接口（不再直接使用，用于降级）
func SetOnline(ctx context.Context, userID string) error {
	return redisClient.SAdd(ctx, OnlineUsersKey(), userID).Err()
}
func SetOffline(ctx context.Context, userID string) error {
	return redisClient.SRem(ctx, OnlineUsersKey(), userID).Err()
}

// SetDeviceOnline/SetDeviceOffline 维护多设备在线状态：
// - 上线：写入用户设备集合 + 全局在线集合
// - 下线：从设备集合移除；若集合为空，则从全局在线集合移除
func SetDeviceOnline(ctx context.Context, userID, deviceID string) error {
	pipe := redisClient.TxPipeline()
	pipe.SAdd(ctx, DevicePresenceKey(userID), deviceID)
	pipe.SAdd(ctx, OnlineUsersKey(), userID)
	_, err := pipe.Exec(ctx)
	return err
}

func SetDeviceOffline(ctx context.Context, userID, deviceID string) error {
	// 先移除设备，再根据剩余设备决定是否从全局在线集合移除
	if err := redisClient.SRem(ctx, DevicePresenceKey(userID), deviceID).Err(); err != nil {
		return err
	}
	if n, err := redisClient.SCard(ctx, DevicePresenceKey(userID)).Result(); err == nil {
		if n == 0 {
			_ = redisClient.SRem(ctx, OnlineUsersKey(), userID).Err()
		}
	}
	return nil
}

// OnlineDeviceCount/OnlineDevices 查询用户的在线设备信息。
func OnlineDeviceCount(ctx context.Context, userID string) (int64, error) {
	return redisClient.SCard(ctx, DevicePresenceKey(userID)).Result()
}

func OnlineDevices(ctx context.Context, userID string) ([]string, error) {
	return redisClient.SMembers(ctx, DevicePresenceKey(userID)).Result()
}
