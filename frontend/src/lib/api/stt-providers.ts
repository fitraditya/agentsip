import { apiRequest } from "./client";
import type { SttProvider, SttProviderRequest } from "./types";

export const sttProvidersApi = {
  async list(): Promise<SttProvider[]> {
    const data = await apiRequest<{ stt_providers: SttProvider[] }>(
      "/stt-providers",
    );
    return data.stt_providers;
  },

  /** Throws `NOT_FOUND` (404) if the provider doesn't exist. */
  async get(id: string): Promise<SttProvider> {
    const data = await apiRequest<{ stt_provider: SttProvider }>(
      `/stt-providers/${id}`,
    );
    return data.stt_provider;
  },

  async create(req: SttProviderRequest): Promise<SttProvider> {
    const data = await apiRequest<{ stt_provider: SttProvider }>(
      "/stt-providers",
      { method: "POST", body: req },
    );
    return data.stt_provider;
  },

  /** Full replace, not a merge. A blank/omitted `api_key` clears it. */
  async update(id: string, req: SttProviderRequest): Promise<SttProvider> {
    const data = await apiRequest<{ stt_provider: SttProvider }>(
      `/stt-providers/${id}`,
      { method: "PUT", body: req },
    );
    return data.stt_provider;
  },

  /** Throws `CONFLICT` (409) if a trunk still references this provider. */
  async delete(id: string): Promise<void> {
    await apiRequest<{ id: string; deleted: boolean }>(
      `/stt-providers/${id}`,
      { method: "DELETE" },
    );
  },
};
