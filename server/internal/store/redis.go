// Package store is the relay's only stateful layer. It knows nothing about
// cryptography — it stores opaque ciphertext bytes and public key strings,
// and enforces TTL / burn-after-reading lifecycle semantics via Redis.
//
// Owner: Pavan (Backend Relay Lead) — server/
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Mob-max30/pandoras-veil/server/internal/idgen"
)

// Sentinel errors returned by store methods. Handlers map these to HTTP
// status codes; they never leak Redis-specific error types upward.
var (
	ErrNotFound    = errors.New("store: not found")
	ErrHandleTaken = errors.New("store: handle already registered")
	ErrUnavailable = errors.New("store: backing store unavailable")
)

// KeyRecord is a registered device public key.
type KeyRecord struct {
	Handle      string `json:"handle"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

// PasteRecord is what actually gets returned to a reader.
type PasteRecord struct {
	Ciphertext string `json:"ciphertext"` // base64, stored exactly as uploaded
}

const (
	keyPrefix   = "pandora:key:"   // pandora:key:<handle>       -> hash{public_key, fingerprint}
	pastePrefix = "pandora:paste:" // pandora:paste:<id>         -> string (base64 ciphertext)
)

// Store wraps a Redis client with Pandora's Veil's domain operations.
type Store struct {
	rdb *redis.Client
}

// New connects to Redis at addr and returns a ready Store. It performs a
// PING to fail fast on misconfiguration rather than failing lazily on the
// first request.
func New(ctx context.Context, addr, password string, db int) (*Store, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("store: connecting to redis at %s: %w", addr, err)
	}

	return &Store{rdb: rdb}, nil
}

// Close releases the underlying Redis connection pool.
func (s *Store) Close() error {
	return s.rdb.Close()
}

// Ping checks liveness of the backing Redis instance (used by /health).
func (s *Store) Ping(ctx context.Context) error {
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// ---- Key registration -------------------------------------------------

// RegisterKey stores a handle -> public key mapping. It fails with
// ErrHandleTaken if the handle is already registered, matching the spec's
// 409 Conflict behavior. Registration never expires — device identities are
// long-lived, unlike pastes.
func (s *Store) RegisterKey(ctx context.Context, handle, publicKey, fingerprint string) error {
	redisKey := keyPrefix + handle

	// HSETNX-based conflict check: set only the first field atomically, then
	// verify we won the race before writing the rest. A short Lua script
	// would be tighter, but HSETNX + verify keeps this dependency-free and
	// is more than sufficient at hackathon/demo scale.
	ok, err := s.rdb.HSetNX(ctx, redisKey, "public_key", publicKey).Result()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !ok {
		return ErrHandleTaken
	}
	if err := s.rdb.HSet(ctx, redisKey, "fingerprint", fingerprint).Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// LookupKey fetches a registered public key by handle.
func (s *Store) LookupKey(ctx context.Context, handle string) (KeyRecord, error) {
	redisKey := keyPrefix + handle

	vals, err := s.rdb.HGetAll(ctx, redisKey).Result()
	if err != nil {
		return KeyRecord{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if len(vals) == 0 {
		return KeyRecord{}, ErrNotFound
	}
	return KeyRecord{
		Handle:      handle,
		PublicKey:   vals["public_key"],
		Fingerprint: vals["fingerprint"],
	}, nil
}

// ---- Paste lifecycle (TTL + burn-after-reading) ------------------------

// PutPaste stores base64 ciphertext under a fresh ID with the given TTL and
// returns that ID. The relay never inspects or validates the ciphertext
// contents — it is opaque bytes as far as the server is concerned.
func (s *Store) PutPaste(ctx context.Context, id string, ciphertextB64 string, ttl time.Duration) error {
	redisKey := pastePrefix + id

	// SET with NX guards against the astronomically unlikely case of an ID
	// collision silently overwriting someone else's secret.
	ok, err := s.rdb.SetNX(ctx, redisKey, ciphertextB64, ttl).Result()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !ok {
		return fmt.Errorf("store: id collision for %s", id)
	}
	return nil
}

// GetPaste fetches ciphertext by ID.
//
// Whether this is a burn-after-reading read is determined entirely by the
// ID itself (see idgen), because GET /paste/:id carries no request body to
// pass that flag explicitly.
//
// For burn IDs, it performs an atomic Redis GETDEL: the value is read and
// deleted in a single server-side operation, so two concurrent readers can
// never both receive the ciphertext — only one GETDEL can win, the other
// always sees a miss. This is the server-side half of MVP-10 (atomic
// burn-after-reading).
//
// For persistent IDs, it performs a plain GET, leaving Redis's own TTL
// eviction to handle expiry (MVP-9).
func (s *Store) GetPaste(ctx context.Context, id string) (PasteRecord, error) {
	redisKey := pastePrefix + id

	var (
		val string
		err error
	)
	if idgen.IsBurnAfterReading(id) {
		val, err = s.rdb.GetDel(ctx, redisKey).Result()
	} else {
		val, err = s.rdb.Get(ctx, redisKey).Result()
	}

	if errors.Is(err, redis.Nil) {
		return PasteRecord{}, ErrNotFound
	}
	if err != nil {
		return PasteRecord{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	return PasteRecord{Ciphertext: val}, nil
}

// ---- Redis Pub/Sub ------------------------------------------------------

// Subscribe returns a Redis PubSub for the given channel.
func (s *Store) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	return s.rdb.Subscribe(ctx, channel)
}

// Publish posts a message to a Redis PubSub channel.
func (s *Store) Publish(ctx context.Context, channel, message string) error {
	return s.rdb.Publish(ctx, channel, message).Err()
}

// ---- Offline Inbox Queueing ---------------------------------------------

const (
	inboxPrefix        = "pandora:inbox:"         // pandora:inbox:<recipient>:<sender> -> list of msg JSON
	inboxSendersPrefix = "pandora:inbox_senders:" // pandora:inbox_senders:<recipient> -> set of senders
)

// PushInboxMessage appends a pending message JSON string to a recipient-sender inbox list with TTL.
func (s *Store) PushInboxMessage(ctx context.Context, recipient, sender, msgJSON string, ttl time.Duration) error {
	redisKey := inboxPrefix + recipient + ":" + sender
	sendersKey := inboxSendersPrefix + recipient
	pipe := s.rdb.Pipeline()
	pipe.RPush(ctx, redisKey, msgJSON)
	pipe.Expire(ctx, redisKey, ttl)
	pipe.SAdd(ctx, sendersKey, sender)
	pipe.Expire(ctx, sendersKey, ttl)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// GetAndClearInbox fetches all pending messages for recipient from sender and clears the inbox list.
func (s *Store) GetAndClearInbox(ctx context.Context, recipient, sender string) ([]string, error) {
	redisKey := inboxPrefix + recipient + ":" + sender
	sendersKey := inboxSendersPrefix + recipient
	msgs, err := s.rdb.LRange(ctx, redisKey, 0, -1).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if len(msgs) > 0 {
		_ = s.rdb.Del(ctx, redisKey).Err()
		_ = s.rdb.SRem(ctx, sendersKey, sender).Err()
	}
	return msgs, nil
}

// GetAllAndClearInbox fetches all pending offline messages across all senders for a recipient.
func (s *Store) GetAllAndClearInbox(ctx context.Context, recipient string) ([]string, error) {
	sendersKey := inboxSendersPrefix + recipient
	senders, err := s.rdb.SMembers(ctx, sendersKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	var allMsgs []string
	for _, sender := range senders {
		msgs, err := s.GetAndClearInbox(ctx, recipient, sender)
		if err == nil && len(msgs) > 0 {
			allMsgs = append(allMsgs, msgs...)
		}
	}
	_ = s.rdb.Del(ctx, sendersKey).Err()

	// Scan pattern fallback to catch any existing unindexed keys
	pattern := inboxPrefix + recipient + ":*"
	keys, err := s.rdb.Keys(ctx, pattern).Result()
	if err == nil {
		for _, key := range keys {
			msgs, err := s.rdb.LRange(ctx, key, 0, -1).Result()
			if err == nil && len(msgs) > 0 {
				allMsgs = append(allMsgs, msgs...)
				_ = s.rdb.Del(ctx, key).Err()
			}
		}
	}

	return allMsgs, nil
}

