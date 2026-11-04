import { apiRequest } from "./client";
import type { Call, ListCallsParams } from "./types";

export const callsApi = {
  async list(params: ListCallsParams = {}): Promise<Call[]> {
    const data = await apiRequest<{ calls: Call[] }>("/calls", {
      query: { limit: params.limit, before: params.before },
    });
    return data.calls;
  },

  /** Throws `NOT_FOUND` (404) if the call doesn't exist. */
  async get(id: string): Promise<Call> {
    const data = await apiRequest<{ call: Call }>(`/calls/${id}`);
    return data.call;
  },

  /**
   * Accepts a pending inbound call waiting on a webhook decision.
   * `pendingCallId` is short-lived/single-use, not a listable resource.
   * Throws `CALL_DECISION_ALREADY_MADE` (409) if already decided/timed out.
   */
  async accept(pendingCallId: string): Promise<void> {
    await apiRequest<{ pending_call_id: string; decision: string }>(
      `/calls/${pendingCallId}/accept`,
      { method: "POST" },
    );
  },

  /** `statusCode` must be a SIP response code 300–699; omit to use the
   * trunk's default (480). */
  async reject(pendingCallId: string, statusCode?: number): Promise<void> {
    await apiRequest<{ pending_call_id: string; decision: string }>(
      `/calls/${pendingCallId}/reject`,
      {
        method: "POST",
        body: statusCode ? { status_code: statusCode } : undefined,
      },
    );
  },
};
