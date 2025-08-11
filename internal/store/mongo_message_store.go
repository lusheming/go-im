package store

import (
	"context"
	"time"

	"go-im/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoMessageStore 基于 MongoDB 的消息存储实现。
// - NewMongoMessageStore 会在 messages 集合上创建 expire_at 的 TTL 索引（partial），到期文档自动清理
// - 通过 (conv_id, client_msg_id) 唯一索引保障幂等
// - List 过滤 recalled 与 expire_at<=now 的消息
// - RecallBySeq 仅作用于 burn_after_read=true 的文档
type MongoMessageStore struct {
	DB *mongo.Database
}

func NewMongoMessageStore(db *mongo.Database) *MongoMessageStore {
	ms := &MongoMessageStore{DB: db}
	// 初始化索引：TTL（expire_at）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// TTL 索引：对存在 expire_at 的文档在到期后自动删除
	_, _ = ms.collection().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "expire_at", Value: 1}},
		Options: options.Index().
			SetExpireAfterSeconds(0).
			SetName("ttl_expire_at").
			SetPartialFilterExpression(bson.D{{Key: "expire_at", Value: bson.D{{Key: "$exists", Value: true}}}}),
	})
	return ms
}

// mongoMessage 为存储层内部结构，与 models.Message 字段一一映射（部分命名略有差异）。
type mongoMessage struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"`
	ServerMsgID   string             `bson:"server_msg_id"`
	ClientMsgID   string             `bson:"client_msg_id"`
	ConvID        string             `bson:"conv_id"`
	ConvType      string             `bson:"conv_type"`
	FromUserID    string             `bson:"from_user_id"`
	ToUserID      string             `bson:"to_user_id,omitempty"`
	GroupID       string             `bson:"group_id,omitempty"`
	Seq           int64              `bson:"seq"`
	Timestamp     time.Time          `bson:"timestamp"`
	Type          string             `bson:"type"`
	Payload       []byte             `bson:"payload"`
	Recalled      bool               `bson:"recalled"`
	StreamID      string             `bson:"stream_id,omitempty"`
	StreamSeq     int                `bson:"stream_seq,omitempty"`
	StreamStatus  string             `bson:"stream_status,omitempty"`
	IsStreaming   bool               `bson:"is_streaming,omitempty"`
	ExpireAt      *time.Time         `bson:"expire_at,omitempty"`
	BurnAfterRead bool               `bson:"burn_after_read,omitempty"`
}

// MongoDB 会话删除水位文档
type mongoConvDelete struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	OwnerID   string             `bson:"owner_id"`
	ConvID    string             `bson:"conv_id"`
	DeletedAt time.Time          `bson:"deleted_at"`
}

func (s *MongoMessageStore) collection() *mongo.Collection {
	return s.DB.Collection("messages")
}

func (s *MongoMessageStore) deleteCollection() *mongo.Collection {
	return s.DB.Collection("conv_deletes")
}

// Append 幂等写入消息（upsert + $setOnInsert）。
func (s *MongoMessageStore) Append(ctx context.Context, m *models.Message) error {
	doc := &mongoMessage{
		ServerMsgID:   m.ServerMsgID,
		ClientMsgID:   m.ClientMsgID,
		ConvID:        m.ConvID,
		ConvType:      string(m.ConvType),
		FromUserID:    m.FromUserID,
		ToUserID:      m.ToUserID,
		GroupID:       m.GroupID,
		Seq:           m.Seq,
		Timestamp:     m.Timestamp,
		Type:          m.Type,
		Payload:       m.Payload,
		Recalled:      m.Recalled,
		StreamID:      m.StreamID,
		StreamSeq:     m.StreamSeq,
		StreamStatus:  m.StreamStatus,
		IsStreaming:   m.IsStreaming,
		ExpireAt:      m.ExpireAt,
		BurnAfterRead: m.BurnAfterRead,
	}

	// 创建唯一索引确保幂等性（容错：重复创建无害）
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "conv_id", Value: 1}, {Key: "client_msg_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_conv_client"),
	}
	s.collection().Indexes().CreateOne(ctx, indexModel)

	// 使用 upsert 实现 INSERT IGNORE 语义
	filter := bson.D{
		{Key: "conv_id", Value: m.ConvID},
		{Key: "client_msg_id", Value: m.ClientMsgID},
	}
	update := bson.D{{Key: "$setOnInsert", Value: doc}}
	opts := options.Update().SetUpsert(true)

	_, err := s.collection().UpdateOne(ctx, filter, update, opts)
	return err
}

// Recall 按 server_msg_id 撤回。
func (s *MongoMessageStore) Recall(ctx context.Context, convID, serverMsgID string) error {
	filter := bson.D{
		{Key: "conv_id", Value: convID},
		{Key: "server_msg_id", Value: serverMsgID},
	}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "recalled", Value: true}}}}

	_, err := s.collection().UpdateOne(ctx, filter, update)
	return err
}

// DeleteConversation 记录删除水位（owner+conv 唯一）。
func (s *MongoMessageStore) DeleteConversation(ctx context.Context, ownerID, convID string) error {
	doc := &mongoConvDelete{
		OwnerID:   ownerID,
		ConvID:    convID,
		DeletedAt: time.Now(),
	}

	// 创建复合唯一索引
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "owner_id", Value: 1}, {Key: "conv_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	s.deleteCollection().Indexes().CreateOne(ctx, indexModel)

	// upsert 实现 ON DUPLICATE KEY UPDATE 语义
	filter := bson.D{
		{Key: "owner_id", Value: ownerID},
		{Key: "conv_id", Value: convID},
	}
	update := bson.D{{Key: "$set", Value: doc}}
	opts := options.Update().SetUpsert(true)

	_, err := s.deleteCollection().UpdateOne(ctx, filter, update, opts)
	return err
}

// List 增量拉取历史：过滤 recalled 与已过期。
func (s *MongoMessageStore) List(ctx context.Context, convID string, fromSeq int64, limit int) ([]*models.Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	filter := bson.D{
		{Key: "conv_id", Value: convID},
		{Key: "seq", Value: bson.D{{Key: "$gt", Value: fromSeq}}},
		{Key: "recalled", Value: false},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "expire_at", Value: bson.D{{Key: "$eq", Value: nil}}}},
			bson.D{{Key: "expire_at", Value: bson.D{{Key: "$gt", Value: time.Now()}}}},
		}},
	}
	opts := options.Find().SetSort(bson.D{{Key: "seq", Value: 1}}).SetLimit(int64(limit))

	cursor, err := s.collection().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var result []*models.Message
	for cursor.Next(ctx) {
		var doc mongoMessage
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		msg := &models.Message{
			ServerMsgID:   doc.ServerMsgID,
			ClientMsgID:   doc.ClientMsgID,
			ConvID:        doc.ConvID,
			ConvType:      models.ConversationType(doc.ConvType),
			FromUserID:    doc.FromUserID,
			ToUserID:      doc.ToUserID,
			GroupID:       doc.GroupID,
			Seq:           doc.Seq,
			Timestamp:     doc.Timestamp,
			Type:          doc.Type,
			Payload:       doc.Payload,
			Recalled:      doc.Recalled,
			StreamID:      doc.StreamID,
			StreamSeq:     doc.StreamSeq,
			StreamStatus:  doc.StreamStatus,
			IsStreaming:   doc.IsStreaming,
			ExpireAt:      doc.ExpireAt,
			BurnAfterRead: doc.BurnAfterRead,
		}
		result = append(result, msg)
	}

	return result, cursor.Err()
}

// DeleteExpired 物理删除到期文档（一般由 TTL 自动处理，此方法作为补充/容错）。
func (s *MongoMessageStore) DeleteExpired(ctx context.Context, before time.Time) error {
	filter := bson.D{{Key: "expire_at", Value: bson.D{{Key: "$ne", Value: nil}, {Key: "$lte", Value: before}}}}
	_, err := s.collection().DeleteMany(ctx, filter)
	return err
}

// RecallBySeq 按 seq 撤回，仅当 burn_after_read=true。
func (s *MongoMessageStore) RecallBySeq(ctx context.Context, convID string, seq int64) error {
	filter := bson.D{{Key: "conv_id", Value: convID}, {Key: "seq", Value: seq}, {Key: "burn_after_read", Value: true}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "recalled", Value: true}}}}
	_, err := s.collection().UpdateOne(ctx, filter, update)
	return err
}

// GetBySeq 查询会话内某条消息（用于阅后即焚判定与撤回广播）。
func (s *MongoMessageStore) GetBySeq(ctx context.Context, convID string, seq int64) (*models.Message, error) {
	filter := bson.D{{Key: "conv_id", Value: convID}, {Key: "seq", Value: seq}}
	var doc mongoMessage
	err := s.collection().FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		return nil, err
	}
	msg := &models.Message{
		ServerMsgID:   doc.ServerMsgID,
		ClientMsgID:   doc.ClientMsgID,
		ConvID:        doc.ConvID,
		ConvType:      models.ConversationType(doc.ConvType),
		FromUserID:    doc.FromUserID,
		ToUserID:      doc.ToUserID,
		GroupID:       doc.GroupID,
		Seq:           doc.Seq,
		Timestamp:     doc.Timestamp,
		Type:          doc.Type,
		Payload:       doc.Payload,
		Recalled:      doc.Recalled,
		StreamID:      doc.StreamID,
		StreamSeq:     doc.StreamSeq,
		StreamStatus:  doc.StreamStatus,
		IsStreaming:   doc.IsStreaming,
		ExpireAt:      doc.ExpireAt,
		BurnAfterRead: doc.BurnAfterRead,
	}
	return msg, nil
}
