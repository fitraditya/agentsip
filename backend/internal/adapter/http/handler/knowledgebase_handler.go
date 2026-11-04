package handler

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/adapter/http/httpx"
	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/pkg/textextract"
	"github.com/fitraditya/agentsip/internal/usecase/knowledgebase"
)

// maxUploadFileSize bounds the raw uploaded file, not the extracted text —
// generous enough for a long scanned report, small enough that one upload
// can't tie up the request for an unreasonable amount of time.
const maxUploadFileSize = 20 << 20 // 20 MiB

var allowedUploadExtensions = map[string]bool{
	".pdf": true,
	".txt": true,
	".md":  true,
}

type KnowledgeBaseHandler struct {
	svc *knowledgebase.Service
}

func NewKnowledgeBaseHandler(svc *knowledgebase.Service) *KnowledgeBaseHandler {
	return &KnowledgeBaseHandler{svc: svc}
}

func (h *KnowledgeBaseHandler) Routes(r chi.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{knowledgeBaseID}", h.get)
	r.Post("/{knowledgeBaseID}/documents", h.ingestDocument)
	r.Post("/{knowledgeBaseID}/documents/upload", h.uploadDocument)
	r.Get("/{knowledgeBaseID}/documents", h.listDocuments)
}

type embeddingConfigDTO struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key,omitempty"`
	Model    string `json:"model"`
}

// vectorStoreConfigDTO is optional on create: an empty/omitted object means
// "use this deployment's platform-managed vector store" (see
// domain.VectorStoreConfig). Bringing your own Postgres sets provider to
// "pgvector" and dsn to its connection string.
type vectorStoreConfigDTO struct {
	Provider string `json:"provider,omitempty"`
	DSN      string `json:"dsn,omitempty"`
}

type knowledgeBaseRequest struct {
	Name        string               `json:"name"`
	Embedding   embeddingConfigDTO   `json:"embedding"`
	VectorStore vectorStoreConfigDTO `json:"vector_store"`
}

// knowledgeBaseResponse omits the embedding API key and the vector store DSN
// (which usually embeds a password), same reasoning as trunkResponse:
// secrets are write-only through this API.
type knowledgeBaseResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Embedding struct {
		Provider         string `json:"provider"`
		BaseURL          string `json:"base_url"`
		Model            string `json:"model"`
		APIKeyConfigured bool   `json:"api_key_configured"`
	} `json:"embedding"`
	VectorStore struct {
		Provider     string `json:"provider"`
		CustomDSNSet bool   `json:"custom_dsn_set"`
	} `json:"vector_store"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toKnowledgeBaseResponse(kb *domain.KnowledgeBase) knowledgeBaseResponse {
	var resp knowledgeBaseResponse
	resp.ID = kb.ID
	resp.Name = kb.Name
	resp.Embedding.Provider = string(kb.Embedding.Provider)
	resp.Embedding.BaseURL = kb.Embedding.BaseURL
	resp.Embedding.Model = kb.Embedding.Model
	resp.Embedding.APIKeyConfigured = kb.Embedding.APIKey != ""
	resp.VectorStore.Provider = string(kb.VectorStore.Provider)
	resp.VectorStore.CustomDSNSet = kb.VectorStore.DSN != ""
	resp.CreatedAt = kb.CreatedAt
	resp.UpdatedAt = kb.UpdatedAt
	return resp
}

func (h *KnowledgeBaseHandler) create(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeEnvelope[knowledgeBaseRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	kb := &domain.KnowledgeBase{
		Name: req.Name,
		Embedding: domain.EmbeddingConfig{
			Provider: domain.ProviderKind(req.Embedding.Provider),
			BaseURL:  req.Embedding.BaseURL,
			APIKey:   req.Embedding.APIKey,
			Model:    req.Embedding.Model,
		},
		VectorStore: domain.VectorStoreConfig{
			Provider: domain.ProviderKind(req.VectorStore.Provider),
			DSN:      req.VectorStore.DSN,
		},
	}
	if err := h.svc.CreateKnowledgeBase(r.Context(), kb); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusCreated, map[string]any{"knowledge_base": toKnowledgeBaseResponse(kb)})
}

func (h *KnowledgeBaseHandler) list(w http.ResponseWriter, r *http.Request) {
	kbs, err := h.svc.ListKnowledgeBases(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	resp := make([]knowledgeBaseResponse, 0, len(kbs))
	for _, kb := range kbs {
		resp = append(resp, toKnowledgeBaseResponse(kb))
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"knowledge_bases": resp})
}

func (h *KnowledgeBaseHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "knowledgeBaseID"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid knowledge base id"}))
		return
	}

	kb, err := h.svc.GetKnowledgeBase(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"knowledge_base": toKnowledgeBaseResponse(kb)})
}

func (h *KnowledgeBaseHandler) listDocuments(w http.ResponseWriter, r *http.Request) {
	kbID, err := uuid.Parse(chi.URLParam(r, "knowledgeBaseID"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid knowledge base id"}))
		return
	}

	docs, err := h.svc.ListDocuments(r.Context(), kbID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	resp := make([]documentResponse, 0, len(docs))
	for _, d := range docs {
		resp = append(resp, toDocumentResponse(d))
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"documents": resp})
}

type ingestDocumentRequest struct {
	SourceURI string `json:"source_uri"`
	Text      string `json:"text"`
}

type documentResponse struct {
	ID              uuid.UUID `json:"id"`
	KnowledgeBaseID uuid.UUID `json:"knowledge_base_id"`
	SourceURI       string    `json:"source_uri"`
	Status          string    `json:"status"`
	FailureReason   *string   `json:"failure_reason,omitempty"`
}

// ingestDocument accepts already-extracted plain text, not a file upload —
// see usecase/knowledgebase.Service's doc comment for why file upload/
// extraction is out of scope for this slice.
func (h *KnowledgeBaseHandler) ingestDocument(w http.ResponseWriter, r *http.Request) {
	kbID, err := uuid.Parse(chi.URLParam(r, "knowledgeBaseID"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid knowledge base id"}))
		return
	}

	req, err := httpx.DecodeEnvelope[ingestDocumentRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}
	if req.Text == "" {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "text is required"}))
		return
	}

	doc, err := h.svc.IngestText(r.Context(), kbID, req.SourceURI, req.Text)
	if err != nil && doc == nil {
		httpx.WriteError(w, err)
		return
	}
	// A non-nil doc with a non-nil err means ingestion reached the document
	// row but failed partway through (e.g. the embedding provider rejected
	// the request). That's still a successful API call — the document
	// exists, with its Status/FailureReason reporting what happened — not a
	// protocol-level error, same reasoning as obrelin returning 201 with a
	// "warning" field when a default quota couldn't be provisioned.
	httpx.WriteSuccess(w, http.StatusCreated, map[string]any{"document": toDocumentResponse(doc)})
}

// uploadDocument accepts a multipart file (.pdf, .txt, or .md), extracts
// and sanitizes its text server-side via internal/pkg/textextract, and
// ingests the result through the same path as ingestDocument. The client
// never sees or handles raw file bytes — this is the only place in the
// codebase that parses an untrusted document format.
func (h *KnowledgeBaseHandler) uploadDocument(w http.ResponseWriter, r *http.Request) {
	kbID, err := uuid.Parse(chi.URLParam(r, "knowledgeBaseID"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid knowledge base id"}))
		return
	}

	// +1MiB of slack for multipart boundaries/field overhead around the
	// file itself, which ParseMultipartForm bounds separately below.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadFileSize+(1<<20))
	if err := r.ParseMultipartForm(maxUploadFileSize); err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{
			"reason": "the upload is too large (max 20MB) or malformed",
		}))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "a file is required"}))
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedUploadExtensions[ext] {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{
			"reason": "unsupported file type — only .pdf, .txt, and .md are accepted",
		}))
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "could not read the uploaded file"}))
		return
	}

	rawText := string(data)
	if ext == ".pdf" {
		rawText, err = textextract.ExtractPDF(data)
		if err != nil {
			httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{
				"reason": "could not extract text from this PDF — it may be scanned/image-only, encrypted, or corrupted",
			}))
			return
		}
	}

	text := textextract.Sanitize(rawText)
	if text == "" {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{
			"reason": "no extractable text was found in this file",
		}))
		return
	}

	sourceURI := r.FormValue("source_uri")
	if sourceURI == "" {
		sourceURI = header.Filename
	}

	doc, err := h.svc.IngestText(r.Context(), kbID, sourceURI, text)
	if err != nil && doc == nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusCreated, map[string]any{"document": toDocumentResponse(doc)})
}

func toDocumentResponse(d *domain.Document) documentResponse {
	return documentResponse{
		ID:              d.ID,
		KnowledgeBaseID: d.KnowledgeBaseID,
		SourceURI:       d.SourceURI,
		Status:          string(d.Status),
		FailureReason:   d.FailureReason,
	}
}
