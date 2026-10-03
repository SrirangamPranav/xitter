package graph

import (
	"context"
	"encoding/json"
	"log"

	"github.com/monis/twitter-backend-go/internal/graph/generated"
	"github.com/monis/twitter-backend-go/internal/graph/model"
	"github.com/monis/twitter-backend-go/internal/pubsub"
)

// TweetAdded streams new tweets matching an optional author filter or all tweets in real time
func (r *subscriptionResolver) TweetAdded(ctx context.Context, authorID *string) (<-chan *model.Tweet, error) {
	channelID := ""
	if authorID != nil {
		channelID = *authorID
	}

	redisChan := pubsub.TweetChannel(channelID)
	rawEvents, cleanup := r.PubSub.Subscribe(ctx, redisChan)
	out := make(chan *model.Tweet, 10)

	go func() {
		defer cleanup()
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case payload, ok := <-rawEvents:
				if !ok {
					return
				}
				var tweet model.Tweet
				if err := json.Unmarshal([]byte(payload), &tweet); err != nil {
					log.Printf("[Subscription] Failed decoding tweet event: %v", err)
					continue
				}
				select {
				case out <- &tweet:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// FeedUpdated streams tweets appearing in the authenticated user's home timeline
func (r *subscriptionResolver) FeedUpdated(ctx context.Context) (<-chan *model.Tweet, error) {
	// Subscribes to global tweet broadcast and filters or delivers real-time updates
	redisChan := pubsub.TweetChannel("")
	rawEvents, cleanup := r.PubSub.Subscribe(ctx, redisChan)
	out := make(chan *model.Tweet, 10)

	go func() {
		defer cleanup()
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case payload, ok := <-rawEvents:
				if !ok {
					return
				}
				var tweet model.Tweet
				if err := json.Unmarshal([]byte(payload), &tweet); err != nil {
					continue
				}
				select {
				case out <- &tweet:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// TweetLiked streams real-time like counts and events for a specific tweet
func (r *subscriptionResolver) TweetLiked(ctx context.Context, tweetID string) (<-chan *model.TweetLikeEvent, error) {
	redisChan := pubsub.TweetLikeChannel(tweetID)
	rawEvents, cleanup := r.PubSub.Subscribe(ctx, redisChan)
	out := make(chan *model.TweetLikeEvent, 10)

	go func() {
		defer cleanup()
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case payload, ok := <-rawEvents:
				if !ok {
					return
				}
				var event model.TweetLikeEvent
				if err := json.Unmarshal([]byte(payload), &event); err != nil {
					continue
				}
				select {
				case out <- &event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

type subscriptionResolver struct{ *Resolver }

func (r *Resolver) Subscription() generated.SubscriptionResolver { return &subscriptionResolver{r} }
