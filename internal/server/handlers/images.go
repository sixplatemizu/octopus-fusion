package handlers

import (
	"net/http"

	"github.com/bestruirui/octopus/internal/relay"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/looplj/axonhub/llm"
)

func init() {
	router.NewGroupRouter("/v1/images").
		Use(middleware.APIKeyAuth()).
		AddRoute(
			router.NewRoute("/generations", http.MethodPost).
				Handle(relay.Forward(llm.APIFormatOpenAIImageGeneration)),
		).
		AddRoute(
			router.NewRoute("/edits", http.MethodPost).
				Handle(relay.Forward(llm.APIFormatOpenAIImageEdit)),
		).
		AddRoute(
			router.NewRoute("/variations", http.MethodPost).
				Handle(relay.Forward(llm.APIFormatOpenAIImageVariation)),
		)
}
