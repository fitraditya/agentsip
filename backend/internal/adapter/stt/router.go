package stt

import (
	"context"
	"fmt"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

type Router struct {
	elevenlabs conversation.STT
	muse       conversation.STT
}

func NewRouter(elevenlabs, muse conversation.STT) *Router {
	return &Router{elevenlabs: elevenlabs, muse: muse}
}

func (r *Router) StartSession(ctx context.Context, cfg conversation.STTSessionConfig) (conversation.STTSession, error) {
	switch cfg.Provider {
	case domain.ProviderMuse:
		if r.muse == nil {
			return nil, fmt.Errorf("stt router: muse provider not configured")
		}
		return r.muse.StartSession(ctx, cfg)
	default:
		if r.elevenlabs == nil {
			return nil, fmt.Errorf("stt router: elevenlabs provider not configured")
		}
		return r.elevenlabs.StartSession(ctx, cfg)
	}
}
