package bootstrap

import (
	"context"
	"log"
	"net/http"

	"example.com/spillway-gate-coordinator/internal/coordination"
	"example.com/spillway-gate-coordinator/internal/httpapi"
	"example.com/spillway-gate-coordinator/internal/storage"
)

func NewHandler() http.Handler {
	repository := storage.NewMemoryRepository()
	service := coordination.NewService(repository)
	if err := service.EnsureBaseline(context.Background()); err != nil {
		log.Fatalf("seed baseline interlock rules: %v", err)
	}
	return httpapi.NewRouter(service)
}
