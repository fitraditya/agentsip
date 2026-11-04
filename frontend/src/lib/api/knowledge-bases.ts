import { apiRequest, apiRequestFormData } from "./client";
import type {
  KnowledgeBase,
  KnowledgeBaseDocument,
  KnowledgeBaseDocumentRequest,
  KnowledgeBaseRequest,
} from "./types";

export const knowledgeBasesApi = {
  async list(): Promise<KnowledgeBase[]> {
    const data = await apiRequest<{ knowledge_bases: KnowledgeBase[] }>(
      "/knowledge-bases",
    );
    return data.knowledge_bases;
  },

  /** Throws `NOT_FOUND` (404) if the knowledge base doesn't exist. */
  async get(id: string): Promise<KnowledgeBase> {
    const data = await apiRequest<{ knowledge_base: KnowledgeBase }>(
      `/knowledge-bases/${id}`,
    );
    return data.knowledge_base;
  },

  async create(req: KnowledgeBaseRequest): Promise<KnowledgeBase> {
    const data = await apiRequest<{ knowledge_base: KnowledgeBase }>(
      "/knowledge-bases",
      { method: "POST", body: req },
    );
    return data.knowledge_base;
  },

  /** Throws `NOT_FOUND` if the knowledge base itself doesn't exist, so an
   * empty array unambiguously means "no documents yet". */
  async listDocuments(kbId: string): Promise<KnowledgeBaseDocument[]> {
    const data = await apiRequest<{ documents: KnowledgeBaseDocument[] }>(
      `/knowledge-bases/${kbId}/documents`,
    );
    return data.documents;
  },

  /**
   * Ingests already-extracted plain text (no file upload/extraction).
   * A 201 can still carry `status: "FAILED"` with `failure_reason` set —
   * that's not a transport error, check `document.status` explicitly.
   */
  async ingestDocument(
    kbId: string,
    req: KnowledgeBaseDocumentRequest,
  ): Promise<KnowledgeBaseDocument> {
    const data = await apiRequest<{ document: KnowledgeBaseDocument }>(
      `/knowledge-bases/${kbId}/documents`,
      { method: "POST", body: req },
    );
    return data.document;
  },

  /**
   * Uploads a .pdf, .txt, or .md file — text extraction (and, for PDFs,
   * sanitization of the extracted content) happens server-side; this
   * client never touches raw file bytes beyond handing them to `fetch`.
   * Same "a 201 can still carry status FAILED" caveat as `ingestDocument`.
   * Throws `VALIDATION_FAILED` for an unsupported extension, an
   * oversized file (>20MB), or a PDF with no extractable text.
   */
  async uploadDocument(
    kbId: string,
    file: File,
    sourceUri?: string,
  ): Promise<KnowledgeBaseDocument> {
    const formData = new FormData();
    formData.set("file", file);
    if (sourceUri) formData.set("source_uri", sourceUri);

    const data = await apiRequestFormData<{ document: KnowledgeBaseDocument }>(
      `/knowledge-bases/${kbId}/documents/upload`,
      formData,
    );
    return data.document;
  },
};
