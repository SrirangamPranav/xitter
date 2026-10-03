package cache_test

import (
	"testing"

	"github.com/monis/twitter-backend-go/internal/cache"
)

func TestCacheKeyGenerators(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected string
	}{
		{
			name:     "TweetKey",
			actual:   cache.TweetKey("123"),
			expected: "cache:tweet:123",
		},
		{
			name:     "UserKey",
			actual:   cache.UserKey("456"),
			expected: "cache:user:456",
		},
		{
			name:     "UserHandleKey",
			actual:   cache.UserHandleKey("jack"),
			expected: "cache:user:handle:jack",
		},
		{
			name:     "FeedKey",
			actual:   cache.FeedKey("789", "cursor1"),
			expected: "cache:feed:789:cursor1",
		},
		{
			name:     "GlobalFeedKey",
			actual:   cache.GlobalFeedKey("cursor2"),
			expected: "cache:feed:global:cursor2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.actual != tt.expected {
				t.Errorf("got %s, expected %s", tt.actual, tt.expected)
			}
		})
	}
}

func TestCacheStatsCalculation(t *testing.T) {
	stats := cache.CacheStats{
		Hits:   80,
		Misses: 20,
		Ratio:  0.8,
	}

	if stats.Ratio != 0.8 {
		t.Errorf("expected ratio 0.8, got %f", stats.Ratio)
	}
}
