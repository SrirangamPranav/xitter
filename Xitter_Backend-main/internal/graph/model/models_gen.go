package model

type User struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Handle         string `json:"handle"`
	DisplayName    string `json:"displayName"`
	AvatarURL      string `json:"avatarUrl"`
	Bio            string `json:"bio"`
	FollowersCount int    `json:"followersCount"`
	FollowingCount int    `json:"followingCount"`
	IsFollowing    bool   `json:"isFollowing"`
	CreatedAt      string `json:"createdAt"`
}

type Tweet struct {
	ID            string  `json:"id"`
	Author        *User   `json:"author"`
	AuthorID      string  `json:"authorId,omitempty"`
	Content       string  `json:"content"`
	ParentTweet   *Tweet  `json:"parentTweet,omitempty"`
	ParentTweetID *string `json:"parentTweetId,omitempty"`
	LikesCount    int     `json:"likesCount"`
	RepliesCount  int     `json:"repliesCount"`
	RetweetsCount int     `json:"retweetsCount"`
	HasLiked      bool    `json:"hasLiked"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

type TweetEdge struct {
	Node   *Tweet `json:"node"`
	Cursor string `json:"cursor"`
}

type PageInfo struct {
	HasNextPage bool    `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor,omitempty"`
}

type TweetConnection struct {
	Edges    []*TweetEdge `json:"edges"`
	PageInfo *PageInfo    `json:"pageInfo"`
}

type TweetLikeEvent struct {
	TweetID    string `json:"tweetId"`
	UserID     string `json:"userId"`
	LikesCount int    `json:"likesCount"`
}

type CreateTweetInput struct {
	Content       string  `json:"content"`
	ParentTweetID *string `json:"parentTweetId,omitempty"`
}

type UpdateProfileInput struct {
	DisplayName *string `json:"displayName,omitempty"`
	Bio         *string `json:"bio,omitempty"`
	AvatarURL   *string `json:"avatarUrl,omitempty"`
}
