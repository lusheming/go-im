package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"go-im/internal/cache"
)

type ReceiptStore struct{ DB *sql.DB }

func NewReceiptStore(db *sql.DB) *ReceiptStore { return &ReceiptStore{DB: db} }

func (s *ReceiptStore) UpsertReadSeq(ctx context.Context, userID, convID string, seq int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO read_receipts(user_id, conv_id, seq) VALUES(?,?,?) ON DUPLICATE KEY UPDATE seq=IF(VALUES(seq) > seq, VALUES(seq), seq)`, userID, convID, seq)
	return err
}

func (s *ReceiptStore) GetReadSeq(ctx context.Context, userID, convID string) (int64, error) {
	var seq sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT seq FROM read_receipts WHERE user_id=? AND conv_id=?`, userID, convID).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if seq.Valid {
		return seq.Int64, nil
	}
	return 0, nil
}

// 标记该用户的所有会话为已读：将 read_receipts 置为 conversations.last_seq（批量事务）
func (s *ReceiptStore) MarkAllReadTx(ctx context.Context, userID string, convIDs []string, lastSeqs map[string]int64) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		} else {
			_ = tx.Commit()
		}
	}()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO read_receipts(user_id, conv_id, seq) VALUES(?,?,?) ON DUPLICATE KEY UPDATE seq=VALUES(seq)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, cid := range convIDs {
		seq := lastSeqs[cid]
		if seq <= 0 {
			continue
		}
		if _, err = stmt.ExecContext(ctx, userID, cid, seq); err != nil {
			return err
		}
		cache.Client().Set(ctx, fmt.Sprintf("im:readseq:%s:%s", userID, cid), seq, 10*time.Minute)
	}
	return nil
}

// Worker 池：对 convIDs 分段并发执行 MarkAllReadTx 事务，带重试
func (s *ReceiptStore) MarkAllReadInChunks(ctx context.Context, userID string, convIDs []string, lastSeqs map[string]int64, chunkSize, concurrency, retry int) error {
	if chunkSize <= 0 {
		chunkSize = 200
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	if retry < 0 {
		retry = 0
	}
	// 切分
	var chunks [][]string
	for i := 0; i < len(convIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(convIDs) {
			end = len(convIDs)
		}
		chunks = append(chunks, convIDs[i:end])
	}
	// 调度
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var firstErr error
	var mu sync.Mutex
	for _, ch := range chunks {
		chCopy := ch
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			var err error
			for attempt := 0; attempt <= retry; attempt++ {
				err = s.MarkAllReadTx(ctx, userID, chCopy, lastSeqs)
				if err == nil {
					break
				}
				time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
			}
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}
