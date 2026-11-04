import { apiRequest } from "./client";
import type { LlmProvider, LlmProviderRequest } from "./types";

export const llmProvidersApi = {
  async list(): Promise<LlmProvider[]> {
    const data = await apiRequest<{ llm_providers: LlmProvider[] }>(
      "/llm-providers",
    );
    return data.llm_providers;
  },

  /** Throws `NOT_FOUND` (404) if the provider doesn't exist. */
  async get(id: string): Promise<LlmProvider> {
    const data = await apiRequest<{ llm_provider: LlmProvider }>(
      `/llm-providers/${id}`,
    );
    return data.llm_provider;
  },

  async create(req: LlmProviderRequest): Promise<LlmProvider> {
    const data = await apiRequest<{ llm_provider: LlmProvider }>(
      "/llm-providers",
      { method: "POST", body: req },
    );
    return data.llm_provider;
  },

  /** Full replace, not a merge. A blank/omitted `api_key` clears it. */
  async update(id: string, req: LlmProviderRequest): Promise<LlmProvider> {
    const data = await apiRequest<{ llm_provider: LlmProvider }>(
      `/llm-providers/${id}`,
      { method: "PUT", body: req },
    );
    return data.llm_provider;
  },

  /** Throws `CONFLICT` (409) if a trunk still references this provider. */
  async delete(id: string): Promise<void> {
    await apiRequest<{ id: string; deleted: boolean }>(
      `/llm-providers/${id}`,
      { method: "DELETE" },
    );
  },
};
