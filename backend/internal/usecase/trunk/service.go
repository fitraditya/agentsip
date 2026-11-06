package trunk

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
)

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, t *domain.Trunk) error {
	if err := validate(t); err != nil {
		return err
	}

	count, err := s.repo.Count(ctx)
	if err != nil {
		return fmt.Errorf("trunk: count: %w", err)
	}
	if count >= domain.MaxTrunks {
		return apperror.ErrTrunkLimitExceeded.WithDetails(map[string]any{
			"current_count": count,
			"max_trunks":    domain.MaxTrunks,
		})
	}

	applyDefaults(t)

	now := time.Now().UTC()
	t.ID = uuid.New()
	t.CreatedAt = now
	t.UpdatedAt = now

	if err := s.repo.Create(ctx, t); err != nil {
		return fmt.Errorf("trunk: create: %w", err)
	}
	return nil
}

func (s *service) Get(ctx context.Context, id uuid.UUID) (*domain.Trunk, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("trunk: get: %w", err)
	}
	return t, nil
}

func (s *service) List(ctx context.Context) ([]*domain.Trunk, error) {
	trunks, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("trunk: list: %w", err)
	}
	return trunks, nil
}

func (s *service) Update(ctx context.Context, t *domain.Trunk) error {
	if err := validate(t); err != nil {
		return err
	}
	// Confirm the trunk exists before writing.
	if _, err := s.repo.Get(ctx, t.ID); err != nil {
		return fmt.Errorf("trunk: update: %w", err)
	}
	applyDefaults(t)
	t.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, t); err != nil {
		return fmt.Errorf("trunk: update: %w", err)
	}
	return nil
}

func (s *service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("trunk: delete: %w", err)
	}
	return nil
}

func (s *service) ResolveInbound(ctx context.Context, match InboundMatch) (*domain.Trunk, error) {
	t, err := s.repo.FindByInboundMatch(ctx, match)
	if err != nil {
		return nil, fmt.Errorf("trunk: resolve inbound: %w", err)
	}
	return t, nil
}

// applyDefaults fills in zero-valued fields that have a meaningful default,
// rather than leaving them as a literal zero that would mean something else
// at the point they're used (e.g. MaxConcurrentCalls == 0 meaning
// "unlimited" to Admission, which is never what an omitted field should mean
// here). Called from both Create and Update so a trunk can't drift back to
// an undefaulted state by being updated with these fields left blank.
func applyDefaults(t *domain.Trunk) {
	if t.MaxConcurrentCalls == 0 {
		t.MaxConcurrentCalls = domain.DefaultMaxConcurrentCalls
	}
	if t.AnswerMode == "" {
		t.AnswerMode = domain.AnswerModeAuto
	}
	if t.AnswerMode == domain.AnswerModeWebhook && t.WebhookTimeoutSeconds == 0 {
		t.WebhookTimeoutSeconds = domain.DefaultWebhookTimeoutSeconds
	}
	// A nil slice (field omitted from the request body entirely) binds as
	// SQL NULL, which violates these columns' NOT NULL constraint — only an
	// explicitly empty array satisfies it. An omitted array and an explicitly
	// empty one mean the same thing here, so normalize nil to empty rather
	// than making every caller remember to send "[]".
	if t.SIPAuth.AllowedIPs == nil {
		t.SIPAuth.AllowedIPs = []string{}
	}
	if t.KnowledgeBaseIDs == nil {
		t.KnowledgeBaseIDs = []uuid.UUID{}
	}
}

func validate(t *domain.Trunk) error {
	reason := ""
	switch {
	case t.Name == "":
		reason = "name is required"
	case t.STTProviderID == uuid.Nil:
		reason = "stt_provider_id is required"
	case t.LLMProviderID == uuid.Nil:
		reason = "llm_provider_id is required"
	case t.TTSProviderID == uuid.Nil:
		reason = "tts_provider_id is required"
	case len(t.SIPAuth.AllowedIPs) == 0 && t.SIPAuth.DigestUsername == "":
		reason = "sip_auth requires at least one allowed IP or digest credentials"
	case t.MaxConcurrentCalls < 0:
		reason = "max_concurrent_calls must not be negative"
	case t.RingDelaySeconds < 0:
		reason = "ring_delay_seconds must not be negative"
	case t.WebhookTimeoutSeconds < 0:
		reason = "webhook_timeout_seconds must not be negative"
	case t.GreetingDelaySeconds < 0:
		reason = "greeting_delay_seconds must not be negative"
	case t.AnswerMode != "" && t.AnswerMode != domain.AnswerModeAuto && t.AnswerMode != domain.AnswerModeWebhook:
		reason = fmt.Sprintf("answer_mode must be %q or %q", domain.AnswerModeAuto, domain.AnswerModeWebhook)
	case t.AnswerMode == domain.AnswerModeWebhook && t.WebhookURL == "":
		reason = fmt.Sprintf("webhook_url is required when answer_mode is %q", domain.AnswerModeWebhook)
	default:
		return nil
	}
	return apperror.ErrValidation.WithDetails(map[string]any{"reason": reason})
}
