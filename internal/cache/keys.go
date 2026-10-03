package cache

import "fmt"

func TweetKey(tweetID string) string {
	return fmt.Sprintf("cache:tweet:%s", tweetID)
}

func UserKey(userID string) string {
	return fmt.Sprintf("cache:user:%s", userID)
}

func UserHandleKey(handle string) string {
	return fmt.Sprintf("cache:user:handle:%s", handle)
}

func FeedKey(userID, cursor string) string {
	return fmt.Sprintf("cache:feed:%s:%s", userID, cursor)
}

func GlobalFeedKey(cursor string) string {
	return fmt.Sprintf("cache:feed:global:%s", cursor)
}

func RepliesKey(tweetID, cursor string) string {
	return fmt.Sprintf("cache:replies:%s:%s", tweetID, cursor)
}
