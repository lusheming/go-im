package mongostore

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Connect 连接 MongoDB
func Connect(uri string) (*mongo.Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	// 测试连接
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	// 从 URI 中提取数据库名，默认为 "goim"
	dbName := "goim"
	if opts := options.Client().ApplyURI(uri); opts.Auth != nil && opts.Auth.AuthSource != "" {
		dbName = opts.Auth.AuthSource
	}
	// 简化：从 URI 路径提取数据库名
	if len(uri) > 10 {
		if idx := findLastSlash(uri); idx > 0 && idx < len(uri)-1 {
			dbName = uri[idx+1:]
		}
	}

	return client.Database(dbName), nil
}

func findLastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}
