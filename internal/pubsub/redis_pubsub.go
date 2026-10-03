package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
)

type EventType string

const (
	EventTweetCreated EventType = "tweet_created"
	EventTweetLiked   EventType = "tweet_liked"
)

type PubSubManager struct {
	client    *redis.Client
	mu        sync.RWMutex
	listeners map[string]map[chan string]struct{}
}

func NewPubSubManager(client *redis.Client) *PubSubManager {
	return &PubSubManager{
		client:    client,
		listeners: make(map[string]map[chan string]struct{}),
	}
}

// Publish broadcasts an event to a Redis channel
func (ps *PubSubManager) Publish(ctx context.Context, channel string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal pubsub payload: %w", err)
	}
	return ps.client.Publish(ctx, channel, string(data)).Err()
}

// Subscribe returns a channel receiving string payloads from a Redis Pub/Sub topic
func (ps *PubSubManager) Subscribe(ctx context.Context, channel string) (<-chan string, func()) {
	out := make(chan string, 100)
	sub := ps.client.Subscribe(ctx, channel)

	go func() {
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				select {
				case out <- msg.Payload:
				default:
					log.Printf("[PubSub] Warning: dropping event on channel %s due to full buffer", channel)
				}
			}
		}
	}()

	cleanup := func() {
		if err := sub.Close(); err != nil {
			log.Printf("[PubSub] Error closing subscription on %s: %v", channel, err)
		}
		close(out)
	}

	return out, cleanup
}

func TweetChannel(authorID string) string {
	if authorID == "" {
		return "events:tweets:all"
	}
	return fmt.Sprintf("events:tweets:author:%s", authorID)
}

func TweetLikeChannel(tweetID string) string {
	return fmt.Sprintf("events:tweets:like:%s", tweetID)
}
