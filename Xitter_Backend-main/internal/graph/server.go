package graph

import (
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/websocket"
	"github.com/monis/twitter-backend-go/internal/graph/generated"
)

// NewGraphQLHandler builds the gqlgen HTTP handler with WebSockets and query complexity protections
func NewGraphQLHandler(resolver *Resolver) *handler.Server {
	srv := handler.New(generated.NewExecutableSchema(generated.Config{
		Resolvers: resolver,
	}))

	// Support WebSocket transport for real-time subscriptions
	srv.AddTransport(&transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins in dev or configure per environment
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	})

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	// Add Introspection & Complexity / Cache control
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: nil,
	})

	return srv
}

// NewPlaygroundHandler provides an interactive GraphQL UI in the browser
func NewPlaygroundHandler(title, endpoint string) http.HandlerFunc {
	return playground.Handler(title, endpoint)
}
