package handler

import (
	"github.com/Naitik2411/go-tasker/internal/server"
	"github.com/Naitik2411/go-tasker/internal/service"
)

type Handlers struct {
	Health  *HealthHandler
	OpenAPI *OpenAPIHandler
}

func NewHandlers(s *server.Server, services *service.Services) *Handlers {
	return &Handlers{}
}
