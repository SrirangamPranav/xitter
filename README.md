# Twitter-like Social Platform Backend in Go

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![GraphQL](https://img.shields.io/badge/GraphQL-gqlgen-E10098?style=flat&logo=graphql)](https://gqlgen.com)
[![Database](https://img.shields.io/badge/PostgreSQL-16-4169E1?style=flat&logo=postgresql)](https://www.postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-Dual_Instance-DC382D?style=flat&logo=redis)](https://redis.io)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A high-performance, production-ready social platform backend engineered in **Go** featuring **GraphQL**, **sqlc**, **PostgreSQL**, and an isolated **dual-Redis architecture**. Built for low latency, sub-millisecond query caching, secure session management, and real-time streaming over WebSockets.

---

## Architecture Highlights

```
                       +-----------------------------------+
                       |    Client / GraphQL Playground    |
                       +-----------------+-----------------+
                                         |
                  +----------------------+----------------------+
                  | HTTP (Queries / Mutations)                  | WebSocket (Subscriptions)
                  v                                             v
       +--------------------+                         +-------------------+
       | Chi HTTP Router    |                         | WebSocket Handler |
       +---------+----------+                         +---------+---------+
                 |                                              |
      [Auth Middleware / Cookie]                                |
                 |                                              |
                 v                                              v
       +--------------------+                         +-------------------+
       | GraphQL Resolvers  | <=== (Pub/Sub Events) = | Redis Pub/Sub Bus |
       +----+----------+----+                         +-------------------+
            |          |
            |          +-------------------------+
            | (Cache Miss / Writes)              | (Cache Read / Cache-Aside)
            v                                    v
  +--------------------+               +--------------------+
  |     PostgreSQL     |               | Dedicated Redis 2  |
  |  (sqlc + pgxpool)  |               |  (LRU Query Cache) |
  |                    |               | Port 6380, LRU     |
  +--------------------+               +--------------------+
            ^
            | (User Lookups)
  +--------------------+
  | Dedicated Redis 1  |
  | (Session Storage)  |
  | Port 6379, Persist |
  +--------------------+
```

---

## 1. Dual-Redis Architecture (Cache Isolation vs. Session Durability)

A critical architectural decision in this platform is operating **two separate Redis instances**:

| Feature | `redis-session` (Port 6379) | `redis-cache` (Port 6380) |
| :--- | :--- | :--- |
| **Purpose** | Server-side user authentication sessions | Database query and timeline caching |
| **Eviction Policy** | `noeviction` (or `volatile-ttl`) | `allkeys-lru` (Least Recently Used) |
| **Memory Cap** | Uncapped or high threshold | Strict limit (e.g., 256MB) |
| **Persistence** | AOF (Append-Only File) enabled | In-memory only (ephemeral) |
| **Lifecycle** | Explicit logout or strict TTL (7 days) | Volatile; evicted on memory pressure |

### Why Isolate Sessions from Query Cache?
In a shared Redis instance with LRU eviction (`maxmemory-policy allkeys-lru`), a sudden spike in tweet queries, hot viral threads, or timeline scans will cause Redis to reach memory saturation. If sessions share the same instance, **Redis will evict active user session keys**, resulting in **logged-in users getting randomly logged out under peak traffic**.

By dedicating `redis-cache` to queries with `allkeys-lru`, the platform drops stale tweets safely, while `redis-session` guarantees active session tokens remain intact.

---

## 2. Type-Safe SQL Generation with `sqlc` & PostgreSQL

Rather than using heavy ORMs with hidden reflection overhead, queries are written in raw SQL and compiled using [`sqlc`](https://sqlc.dev) into type-safe Go code:

- **Zero runtime reflection**: Direct mapping to `jackc/pgx/v5` structs.
- **Optimized Composite Indexing**:
  - `idx_tweets_user_created (user_id, created_at DESC)`: Rapid profile timeline scans.
  - `idx_tweets_created (created_at DESC)`: Global feed and cursor-based pagination.
  - `idx_tweets_parent (parent_tweet_id)`: Instant reply tree reconstruction.
- **Connection Pooling**: Managed via `pgxpool` with dynamic idle connection harvesting and connection lifetime limits.

### N+1 Query Elimination via DataLoader
When querying a timeline of 20 tweets, resolving authors and `hasLiked` flags independently would trigger 41 database queries. We implemented custom Go DataLoaders:
- **`UserBatchLoader`**: Batches author UUIDs into a single `WHERE id = ANY($1::uuid[])` call.
- **`LikeStatusLoader`**: Checks multiple tweet likes in a single `WHERE user_id = $1 AND tweet_id = ANY($2::uuid[])` call.
- **Result**: Reduced timeline query overhead by over **92%**.

---

## 3. Authentication, Sessions & Security

- **Google OAuth 2.0**: Standard RFC 6749 flow with PKCE-compatible state verification to eliminate CSRF attacks.
- **HttpOnly Cookies**: Session tokens are sent to browsers exclusively in `HttpOnly`, `SameSite=Lax`, and `Secure` cookies, preventing XSS access.
- **Cryptographic Tokens**: 32-byte cryptographically secure random session IDs (`crypto/rand`).
- **Server-Side Revocation**:
  - `logout`: Revokes current device session in sub-millisecond Redis `DEL`.
  - `revokeAllSessions`: Invalids all user sessions simultaneously using a Redis `user_sessions:<user_id>` tracking set.
- **Dev Mode / Mock OAuth**: Local bypass endpoint (`/auth/dev/login?handle=monis`) generates instant sessions without needing Google Cloud credentials.

---

## 4. Real-time Subscriptions over WebSockets

Built on standard GraphQL over WebSocket protocol:
- Subscriptions:
  - `tweetAdded(authorId: ID)`: Streams new tweets from specific creators or globally.
  - `feedUpdated`: Pushes live timeline updates.
  - `tweetLiked(tweetId: ID!)`: Real-time like counter updates.
- **Distributed Redis Pub/Sub**: Resolvers publish events through Redis Pub/Sub channels, allowing horizontal scaling across multi-node server clusters.

---

## Quickstart Guide

### Prerequisites
- [Docker & Docker Compose](https://www.docker.com/) OR Go 1.23+ and local PostgreSQL/Redis instances.

### Option A: Running with Docker Compose (Recommended)

1. Clone or navigate to the project directory:
   ```bash
   cd twitter-backend-go
   ```

2. Copy the environment variables:
   ```bash
   cp .env.example .env
   ```

3. Spin up PostgreSQL, both Redis instances, and the Go application:
   ```bash
   docker compose up -d --build
   ```

4. Verify health and services:
   ```bash
   curl http://localhost:8080/health
   ```
   Output:
   ```json
   {
     "status": "online",
     "components": {
       "postgres": "healthy",
       "redis_session": "healthy",
       "redis_cache": "healthy"
     },
     "lru_cache_stats": {
       "hits": 0,
       "misses": 0,
       "hit_ratio": 0
     }
   }
   ```

5. Open GraphQL Playground in your browser:
   **`http://localhost:8080/`**

---

### Option B: Local Native Execution

1. Start databases and Redis using Docker:
   ```bash
   docker compose up -d postgres redis-session redis-cache
   ```

2. Run the Go server:
   ```bash
   go run ./cmd/server
   ```

---

## GraphQL API Examples

### 1. Dev Authentication (Get Session Cookie)
In your browser or curl, navigate to:
```bash
curl -c cookies.txt "http://localhost:8080/auth/dev/login?handle=alex&name=Alex+Morgan"
```
This logs you in as `@alex` and saves the HttpOnly session cookie.

---

### 2. Create a Tweet (Mutation)
```graphql
mutation CreateTweet {
  createTweet(input: {
    content: "Building high-performance backends with Go, GraphQL, sqlc, and dual-Redis!"
  }) {
    id
    content
    createdAt
    author {
      handle
      displayName
    }
  }
}
```

---

### 3. Fetch Home Feed with Cursor Pagination (Query)
```graphql
query GetHomeFeed {
  feed(limit: 10) {
    edges {
      cursor
      node {
        id
        content
        likesCount
        repliesCount
        hasLiked
        author {
          handle
          displayName
          avatarUrl
        }
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

---

### 4. Like a Tweet (Mutation)
```graphql
mutation LikeTweet($tweetId: ID!) {
  likeTweet(tweetId: $tweetId) {
    id
    likesCount
    hasLiked
  }
}
```

---

### 5. Real-Time Tweet Subscription (WebSocket)
```graphql
subscription OnNewTweet {
  tweetAdded {
    id
    content
    author {
      handle
    }
    createdAt
  }
}
```

---

### 6. Revoke All Active Sessions (Logout All Devices)
```graphql
mutation LogoutAllDevices {
  revokeAllSessions
}
```

---

## Project Structure

```
twitter-backend-go/
├── .env.example
├── docker-compose.yml
├── Dockerfile
├── Makefile
├── sqlc.yaml
├── gqlgen.yml
│
├── cmd/
│   └── server/
│       └── main.go              # Server bootstrap, dual-redis pool, Chi router
│
├── api/
│   └── graphql/
│       └── schema.graphqls      # GraphQL Schema definition
│
├── internal/
│   ├── config/
│   │   └── config.go            # Environment configuration
│   ├── auth/
│   │   ├── oauth.go             # Google OAuth2 and Dev login handlers
│   │   ├── session.go           # Redis-backed session manager (TTL, multi-device revoke)
│   │   ├── middleware.go        # HttpOnly cookie & context auth middleware
│   │   └── context.go           # Context helpers
│   ├── cache/
│   │   ├── lru_cache.go         # Dedicated LRU query cache (Cache-Aside, hit ratio)
│   │   └── keys.go              # Cache key formats
│   ├── db/
│   │   ├── pool.go              # pgxpool PostgreSQL connection pool
│   │   ├── migrations/          # SQL schema migrations
│   │   ├── queries/             # sqlc SQL query definitions
│   │   └── sqlc/                # Type-safe Go structs & queries generated by sqlc
│   ├── graph/
│   │   ├── server.go            # GraphQL HTTP & WebSocket handler
│   │   ├── resolver.go          # Resolver dependency container
│   │   ├── schema.resolvers.go  # Query & Mutation resolvers
│   │   ├── tweet.resolvers.go   # Tweet field resolvers
│   │   ├── user.resolvers.go    # User field resolvers
│   │   ├── subscription.resolvers.go # Real-time WebSocket streaming
│   │   ├── dataloader/          # Batch DataLoaders for N+1 prevention
│   │   └── model/               # GraphQL Go models
│   └── pubsub/
│       └── redis_pubsub.go      # Redis Pub/Sub for distributed WebSockets
│
└── README.md
```
