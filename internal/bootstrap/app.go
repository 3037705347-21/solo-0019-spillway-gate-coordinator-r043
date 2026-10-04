package bootstrap

import (
	"context"
	"fmt"
	"net/http"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/coordination"
	"example.com/spillway-gate-coordinator/internal/httpapi"
	"example.com/spillway-gate-coordinator/internal/storage"
)

func NewHandler() http.Handler {
	repository := storage.NewMemoryRepository()
	service := coordination.NewService(repository)
	seedBaselineRule(service)
	return httpapi.NewRouter(service)
}

// seedBaselineRule installs the initial open-ended rule version so that every
// review resolves an effective, readable rule basis from process start. The
// baseline adds no limits beyond the gate's own aperture bounds.
func seedBaselineRule(service *coordination.Service) {
	_, err := service.PublishRule(context.Background(), contracts.PublishInterlockRuleRequest{
		Reason:   "初始基线规则：不施加额外联锁限制",
		IssuedBy: "system",
	})
	if err != nil {
		panic(fmt.Sprintf("seed baseline interlock rule: %v", err))
	}
}
