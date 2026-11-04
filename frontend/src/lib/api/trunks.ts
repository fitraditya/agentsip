import { apiRequest } from "./client";
import type { Trunk, TrunkRequest } from "./types";

export const trunksApi = {
  async list(): Promise<Trunk[]> {
    const data = await apiRequest<{ trunks: Trunk[] }>("/trunks");
    return data.trunks;
  },

  /** Throws `NOT_FOUND` (404) if the trunk doesn't exist. */
  async get(id: string): Promise<Trunk> {
    const data = await apiRequest<{ trunk: Trunk }>(`/trunks/${id}`);
    return data.trunk;
  },

  /** Throws `TRUNK_LIMIT_EXCEEDED` (409) once the deployment already has 3 trunks. */
  async create(req: TrunkRequest): Promise<Trunk> {
    const data = await apiRequest<{ trunk: Trunk }>("/trunks", {
      method: "POST",
      body: req,
    });
    return data.trunk;
  },

  /** Full replace, not a merge — `req` must contain every field, including
   * ones you want to keep unchanged. A blank/omitted secret clears it. */
  async update(id: string, req: TrunkRequest): Promise<Trunk> {
    const data = await apiRequest<{ trunk: Trunk }>(`/trunks/${id}`, {
      method: "PUT",
      body: req,
    });
    return data.trunk;
  },

  async delete(id: string): Promise<void> {
    await apiRequest<{ id: string; deleted: boolean }>(`/trunks/${id}`, {
      method: "DELETE",
    });
  },
};
