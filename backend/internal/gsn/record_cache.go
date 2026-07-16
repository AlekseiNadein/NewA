package gsn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"nav-saas-mvp/backend/internal/observability"
)

const DefaultRecordCacheTTL = 6 * time.Hour

type recordDetailCache struct {
	client *redis.Client
	ttl    time.Duration
}

// ConfigureRedisRecordCache enables Redis caching for GetRecordDetail when enabled is true.
// When disabled, GetRecordDetail reads directly from the database.
// On connection failure the service continues without cache.
func ConfigureRedisRecordCache(s *Service, enabled bool, redisURL string, ttl time.Duration) {
	if s == nil || !s.Configured() {
		return
	}
	if !enabled {
		slog.Info("GSN record cache disabled by configuration")
		return
	}
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		slog.Info("GSN record cache disabled: APP_REDIS_URL is empty")
		return
	}
	if err := s.enableRedisRecordCache(redisURL, ttl); err != nil {
		slog.Warn("GSN redis cache disabled", "error", err)
	}
}

func (s *Service) enableRedisRecordCache(redisURL string, ttl time.Duration) error {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return fmt.Errorf("redis ping: %w", err)
	}
	if ttl <= 0 {
		ttl = DefaultRecordCacheTTL
	}
	s.recordCache = &recordDetailCache{
		client: client,
		ttl:    ttl,
	}
	slog.Info("GSN record cache enabled", "ttl", ttl.String())
	return nil
}

func recordDetailCacheKey(code, fgisSetID, district string) string {
	return fmt.Sprintf("gsn:detail:%s:%s:%s",
		ExtractPositionCipher(code),
		strings.TrimSpace(fgisSetID),
		normalizeDistrictKey(district),
	)
}

func (c *recordDetailCache) get(ctx context.Context, key string) (RecordDetail, bool) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			observability.RecordGSNCacheMiss()
			return RecordDetail{}, false
		}
		observability.RecordGSNCacheError()
		slog.Warn("gsn record cache get failed", "key", key, "error", err)
		return RecordDetail{}, false
	}

	var detail RecordDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		observability.RecordGSNCacheError()
		slog.Warn("gsn record cache decode failed", "key", key, "error", err)
		return RecordDetail{}, false
	}
	observability.RecordGSNCacheHit()
	return detail, true
}

func (c *recordDetailCache) set(ctx context.Context, key string, detail RecordDetail) {
	data, err := json.Marshal(detail)
	if err != nil {
		observability.RecordGSNCacheError()
		slog.Warn("gsn record cache encode failed", "key", key, "error", err)
		return
	}
	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		observability.RecordGSNCacheError()
		slog.Warn("gsn record cache set failed", "key", key, "error", err)
	}
}

func (c *recordDetailCache) close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
