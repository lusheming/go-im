package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-im/internal/config"
	"go-im/internal/store"
	"go-im/internal/store/sqlstore"

	"github.com/IBM/sarama"
)

type groupUpdate struct {
	GroupID string `json:"groupId"`
	ConvID  string `json:"convId"`
	From    string `json:"from"`
	Type    string `json:"type"`
	TS      int64  `json:"ts"`
}

type handler struct {
	ctx        context.Context
	cancel     context.CancelFunc
	groupStore *store.GroupStore
	convStore  *store.ConversationStore
	batchSize  int
	batchSleep time.Duration
}

func (h *handler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (h *handler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }
func (h *handler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		var evt groupUpdate
		if err := json.Unmarshal(msg.Value, &evt); err == nil {
			ids, err := h.groupStore.ListMemberIDs(h.ctx, evt.GroupID)
			if err == nil {
				batch := h.batchSize
				if batch <= 0 {
					batch = 500
				}
				sleep := h.batchSleep
				if sleep <= 0 {
					sleep = 50 * time.Millisecond
				}
				for i := 0; i < len(ids); i += batch {
					end := i + batch
					if end > len(ids) {
						end = len(ids)
					}
					for _, uid := range ids[i:end] {
						_ = h.convStore.UpsertUserConversation(h.ctx, uid, evt.ConvID, evt.Type, evt.From, evt.GroupID)
					}
					time.Sleep(sleep)
				}
			}
		}
		sess.MarkMessage(msg, "")
	}
	return nil
}

func main() {
	cfg := config.Load()
	if cfg.KafkaBrokers == "" {
		log.Fatal("IM_KAFKA_BROKERS 未配置")
	}

	primaryDB := mustOpen(cfg.MySQLDSN)
	groupStore := store.NewGroupStore(primaryDB)
	convStore := store.NewConversationStore(primaryDB)

	ctx, cancel := context.WithCancel(context.Background())
	h := &handler{ctx: ctx, cancel: cancel, groupStore: groupStore, convStore: convStore, batchSize: cfg.GroupBatchSize, batchSleep: time.Duration(cfg.GroupBatchSleepMS) * time.Millisecond}

	client, err := sarama.NewConsumerGroup(splitCSV(cfg.KafkaBrokers), "im-group-consumer", sarama.NewConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	topic := cfg.KafkaGroupUpdateTopic
	go func() {
		for {
			if err := client.Consume(ctx, []string{topic}, h); err != nil {
				log.Printf("consume error: %v", err)
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()
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

func splitCSV(s string) []string {
	var out []string
	for _, p := range []rune(s) {
		_ = p
	}
	// simple split without strings import reuse
	var cur string
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		} else {
			cur += string(s[i])
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
