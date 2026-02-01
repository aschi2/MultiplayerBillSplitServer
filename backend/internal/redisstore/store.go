package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/aschi2/MultiplayerBillSplit/backend/internal/crdt"
)

type Store struct {
	Client *redis.Client
	TTL    time.Duration
}

func New(client *redis.Client, ttl time.Duration) *Store {
	return &Store{Client: client, TTL: ttl}
}

func (s *Store) roomKey(roomID string) string {
	return fmt.Sprintf("room:%s", roomID)
}

func (s *Store) snapshotKey(roomID string) string {
	return fmt.Sprintf("room:%s:snapshot", roomID)
}

func (s *Store) seqKey(roomID string) string {
	return fmt.Sprintf("room:%s:seq", roomID)
}

func (s *Store) opsKey(roomID string) string {
	return fmt.Sprintf("room:%s:ops", roomID)
}

func (s *Store) LoadSnapshot(ctx context.Context, roomID string) (*crdt.RoomDoc, int64, error) {
	payload, err := s.Client.Get(ctx, s.snapshotKey(roomID)).Result()
	if err == redis.Nil {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var doc crdt.RoomDoc
	if err := json.Unmarshal([]byte(payload), &doc); err != nil {
		return nil, 0, err
	}
	seq, _ := s.Client.Get(ctx, s.seqKey(roomID)).Int64()
	return &doc, seq, nil
}

func (s *Store) SaveSnapshot(ctx context.Context, roomID string, doc *crdt.RoomDoc, seq int64) error {
	payload, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	pipe := s.Client.TxPipeline()
	pipe.Set(ctx, s.snapshotKey(roomID), payload, s.TTL)
	pipe.Set(ctx, s.seqKey(roomID), seq, s.TTL)
	pipe.Expire(ctx, s.opsKey(roomID), s.TTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Store) AppendOp(ctx context.Context, roomID string, op crdt.Op) (int64, error) {
	seq, err := s.Client.Incr(ctx, s.seqKey(roomID)).Result()
	if err != nil {
		return 0, err
	}
	wrapper := map[string]any{
		"seq": seq,
		"op":  op,
	}
	payload, err := json.Marshal(wrapper)
	if err != nil {
		return 0, err
	}
	pipe := s.Client.TxPipeline()
	pipe.RPush(ctx, s.opsKey(roomID), payload)
	pipe.Expire(ctx, s.opsKey(roomID), s.TTL)
	pipe.Expire(ctx, s.seqKey(roomID), s.TTL)
	pipe.Exec(ctx)
	return seq, nil
}

// CurrentSeq returns the latest sequence value for a room (or 0 if missing).
func (s *Store) CurrentSeq(ctx context.Context, roomID string) (int64, error) {
	val, err := s.Client.Get(ctx, s.seqKey(roomID)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

func (s *Store) LoadOps(ctx context.Context, roomID string, fromSeq int64) ([]crdt.Op, error) {
	values, err := s.Client.LRange(ctx, s.opsKey(roomID), 0, -1).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ops := []crdt.Op{}
	for _, value := range values {
		var wrapper struct {
			Seq int64   `json:"seq"`
			Op  crdt.Op `json:"op"`
		}
		if err := json.Unmarshal([]byte(value), &wrapper); err != nil {
			continue
		}
		if wrapper.Seq > fromSeq {
			ops = append(ops, wrapper.Op)
		}
	}
	return ops, nil
}

func (s *Store) TouchRoom(ctx context.Context, roomID string) {
	pipe := s.Client.TxPipeline()
	pipe.Expire(ctx, s.snapshotKey(roomID), s.TTL)
	pipe.Expire(ctx, s.seqKey(roomID), s.TTL)
	pipe.Expire(ctx, s.opsKey(roomID), s.TTL)
	pipe.Exec(ctx)
}
