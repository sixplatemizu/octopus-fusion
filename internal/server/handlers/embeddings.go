package handlers

import (
	"net/http"

	"github.com/bestruirui/octopus/internal/relay"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/router"
)

func init() {
	router.NewGroupRouter("/v1").
		Use(middleware.APIKeyAuth()).
		AddRoute(
			router.NewRoute("/embeddings", http.MethodPost).
				Handle(relay.HandleEmbeddings),
		)
}
