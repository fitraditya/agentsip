import { apiRequest } from "./client";
import type { TtsProvider, TtsProviderRequest } from "./types";

export const ttsProvidersApi = {
  async list(): Promise<TtsProvider[]> {
    const data = await apiRequest<{ tts_providers: TtsProvider[] }>(
      "/tts-providers",
    );
    return data.tts_providers;
  },

  /** Throws `NOT_FOUND` (404) if the provider doesn't exist. */
  async get(id: string): Promise<TtsProvider> {
    const data = await apiRequest<{ tts_provider: TtsProvider }>(
      `/tts-providers/${id}`,
    );
    return data.tts_provider;
  },

  async create(req: TtsProviderRequest): Promise<TtsProvider> {
    const data = await apiRequest<{ tts_provider: TtsProvider }>(
      "/tts-providers",
      { method: "POST", body: req },
    );
    return data.tts_provider;
  },

  /** Full replace, not a merge. A blank/omitted `api_key` clears it. */
  async update(id: string, req: TtsProviderRequest): Promise<TtsProvider> {
    const data = await apiRequest<{ tts_provider: TtsProvider }>(
      `/tts-providers/${id}`,
      { method: "PUT", body: req },
    );
    return data.tts_provider;
  },

  /** Throws `CONFLICT` (409) if a trunk still references this provider. */
  async delete(id: string): Promise<void> {
    await apiRequest<{ id: string; deleted: boolean }>(
      `/tts-providers/${id}`,
      { method: "DELETE" },
    );
  },
};
