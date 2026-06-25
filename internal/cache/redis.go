package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leykun/bybit-backtester/internal/engine"
	"github.com/redis/go-redis/v9"
)

const keyVersion = "v1"

type Client struct {
	rdb       *redis.Client
	barsTTL   time.Duration
	resultTTL time.Duration
}

func Connect(ctx context.Context, redisURL string, barsTTL, resultTTL time.Duration) (*Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &Client{rdb: rdb, barsTTL: barsTTL, resultTTL: resultTTL}, nil
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func BarsKey(symbol, interval string, start, end time.Time) string {
	return fmt.Sprintf("bt:%s:bars:%s:%s:%d:%d", keyVersion, symbol, interval, start.Unix(), end.Unix())
}

func resultKey(hash string) string {
	return fmt.Sprintf("bt:%s:result:%s", keyVersion, hash)
}

func (c *Client) GetBars(ctx context.Context, key string) ([]engine.Bar, bool, error) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var bars []engine.Bar
	if err := json.Unmarshal(b, &bars); err != nil {
		return nil, false, err
	}
	if len(bars) == 0 {
		return nil, false, nil
	}
	return bars, true, nil
}

func (c *Client) SetBars(ctx context.Context, key string, bars []engine.Bar) error {
	b, err := json.Marshal(bars)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, b, c.barsTTL).Err()
}

func (c *Client) GetResultRunID(ctx context.Context, requestHash string) (string, bool, error) {
	id, err := c.rdb.Get(ctx, resultKey(requestHash)).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, id != "", nil
}

func (c *Client) SetResultRunID(ctx context.Context, requestHash, runID string) error {
	return c.rdb.Set(ctx, resultKey(requestHash), runID, c.resultTTL).Err()
}
