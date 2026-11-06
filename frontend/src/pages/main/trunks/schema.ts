import type { Trunk, TrunkRequest } from "@/lib/api";
import { z } from "zod";

/**
 * Flat form shape (not nested like `TrunkRequest`) so RHF field names and
 * `Field`/`FieldError` wiring stay simple. `trunkFormToRequest` reassembles
 * this into the `TrunkRequest` the API expects.
 *
 * STT/LLM/TTS are now standalone provider resources (see
 * `sttProvidersApi`/`llmProvidersApi`/`ttsProvidersApi`) — the trunk only
 * holds a reference (`*_provider_id`) to one of each, picked from a select
 * populated by the resource list endpoints.
 *
 * Numeric-looking fields are kept as raw strings and parsed on submit —
 * avoids zod's `coerce.number()` turning a blank optional field into `0`.
 */
export const trunkFormSchema = z
  .object({
    name: z.string().trim().min(1, "Name is required"),
    // Optional: an empty list means "no source-IP restriction" — the trunk
    // then matches on SIP digest auth alone (or, with both left empty,
    // matches any inbound call, which is a valid catch-all/default-trunk
    // configuration the backend explicitly supports — see
    // trunk.FindByInboundMatch).
    allowed_ips: z.string().trim(),
    digest_username: z.string().trim(),
    digest_password: z.string().optional(),
    stt_provider_id: z.string().min(1, "STT provider is required"),
    llm_provider_id: z.string().min(1, "LLM provider is required"),
    tts_provider_id: z.string().min(1, "TTS provider is required"),
    system_prompt: z.string().trim().min(1, "System prompt is required"),
    knowledge_base_ids: z.array(z.string()),
    max_concurrent_calls: z.string().optional(),
    answer_mode: z.enum(["auto", "webhook"]),
    ring_delay_seconds: z.string().optional(),
    webhook_url: z.string().optional(),
    webhook_timeout_seconds: z.string().optional(),
    greeting_text: z.string().optional(),
    greeting_delay_seconds: z.string().optional(),
  })
  .superRefine((data, ctx) => {
    if (data.answer_mode === "webhook" && !data.webhook_url?.trim()) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Webhook URL is required in webhook mode",
        path: ["webhook_url"],
      });
    }
  });

export type TrunkFormData = z.infer<typeof trunkFormSchema>;

function parseIntOrUndefined(value?: string): number | undefined {
  if (!value || !value.trim()) return undefined;
  const parsed = Number.parseInt(value, 10);
  return Number.isNaN(parsed) ? undefined : parsed;
}

export function emptyTrunkFormDefaults(): TrunkFormData {
  return {
    name: "",
    allowed_ips: "",
    digest_username: "",
    digest_password: "",
    stt_provider_id: "",
    llm_provider_id: "",
    tts_provider_id: "",
    system_prompt: "",
    knowledge_base_ids: [],
    max_concurrent_calls: "",
    answer_mode: "auto",
    ring_delay_seconds: "",
    webhook_url: "",
    webhook_timeout_seconds: "",
    greeting_text: "",
    greeting_delay_seconds: "",
  };
}

/** Secrets always come back blank — the GET response never echoes them. */
export function trunkToFormDefaults(trunk: Trunk): TrunkFormData {
  return {
    name: trunk.name,
    allowed_ips: trunk.sip_auth.allowed_ips.join(", "),
    digest_username: trunk.sip_auth.digest_username,
    digest_password: "",
    stt_provider_id: trunk.stt_provider_id,
    llm_provider_id: trunk.llm_provider_id,
    tts_provider_id: trunk.tts_provider_id,
    system_prompt: trunk.system_prompt,
    knowledge_base_ids: trunk.knowledge_base_ids,
    max_concurrent_calls: String(trunk.max_concurrent_calls ?? ""),
    answer_mode: trunk.answer_mode,
    ring_delay_seconds: String(trunk.ring_delay_seconds ?? ""),
    webhook_url: trunk.webhook_url ?? "",
    webhook_timeout_seconds:
      trunk.webhook_timeout_seconds !== undefined
        ? String(trunk.webhook_timeout_seconds)
        : "",
    greeting_text: trunk.greeting_text ?? "",
    greeting_delay_seconds:
      trunk.greeting_delay_seconds !== undefined
        ? String(trunk.greeting_delay_seconds)
        : "",
  };
}

/**
 * Builds the full-replace `TrunkRequest` body. A blank `digest_password` is
 * omitted here, which — per the backend's full-replace (not merge) PUT
 * semantics — clears it. There is no way to "leave it as-is" on update; this
 * is a confirmed backend limitation, not something fixable client-side. The
 * UI must (and does) warn about this before submit.
 */
export function trunkFormToRequest(data: TrunkFormData): TrunkRequest {
  const allowed_ips = data.allowed_ips
    .split(",")
    .map((ip) => ip.trim())
    .filter(Boolean);

  const req: TrunkRequest = {
    name: data.name.trim(),
    sip_auth: {
      allowed_ips,
      digest_username: data.digest_username.trim(),
      ...(data.digest_password
        ? { digest_password: data.digest_password }
        : {}),
    },
    stt_provider_id: data.stt_provider_id,
    llm_provider_id: data.llm_provider_id,
    tts_provider_id: data.tts_provider_id,
    system_prompt: data.system_prompt.trim(),
    knowledge_base_ids: data.knowledge_base_ids,
    answer_mode: data.answer_mode,
  };

  const maxConcurrentCalls = parseIntOrUndefined(data.max_concurrent_calls);
  if (maxConcurrentCalls !== undefined) {
    req.max_concurrent_calls = maxConcurrentCalls;
  }

  if (data.answer_mode === "auto") {
    const ringDelay = parseIntOrUndefined(data.ring_delay_seconds);
    if (ringDelay !== undefined) req.ring_delay_seconds = ringDelay;
  } else {
    req.webhook_url = data.webhook_url?.trim();
    const timeout = parseIntOrUndefined(data.webhook_timeout_seconds);
    if (timeout !== undefined) req.webhook_timeout_seconds = timeout;
  }

  if (data.greeting_text?.trim()) {
    req.greeting_text = data.greeting_text.trim();
  } else if (data.greeting_text === "") {
    // Explicit empty string clears greeting
    req.greeting_text = "";
  }
  const greetingDelay = parseIntOrUndefined(data.greeting_delay_seconds);
  if (greetingDelay !== undefined) req.greeting_delay_seconds = greetingDelay;

  return req;
}
