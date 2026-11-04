import type { LlmProvider, LlmProviderRequest, ProviderKind } from "@/lib/api";
import { z } from "zod";

/** LLM only supports OpenAI-compatible endpoints today — the type allows
 * the full `ProviderKind` union, but the UI only ever offers this one
 * choice. */
export const llmProviderOptions: { value: ProviderKind; label: string }[] = [
  { value: "openai_compatible", label: "OpenAI-compatible" },
];

const providerEnum = z.enum(["openai_compatible"]);

export const llmProviderFormSchema = z.object({
  name: z.string().trim().min(1, "Name is required"),
  provider: providerEnum,
  base_url: z.string().trim().min(1, "Base URL is required"),
  api_key: z.string().optional(),
  model: z.string().trim().min(1, "Model is required"),
});

export type LlmProviderFormData = z.infer<typeof llmProviderFormSchema>;

export function emptyLlmProviderFormDefaults(): LlmProviderFormData {
  return {
    name: "",
    provider: "openai_compatible",
    base_url: "",
    api_key: "",
    model: "",
  };
}

/** Secrets never come back — the GET response never echoes `api_key`. */
export function llmProviderToFormDefaults(
  provider: LlmProvider,
): LlmProviderFormData {
  return {
    name: provider.name,
    // The backend's ProviderKind is wider than this form's single-option
    // enum; narrowed here since "openai_compatible" is the only value the
    // UI ever writes back.
    provider: provider.provider as "openai_compatible",
    base_url: provider.base_url,
    api_key: "",
    model: provider.model,
  };
}

/**
 * Builds the full-replace `LlmProviderRequest` body. A blank `api_key` is
 * omitted here, which — per the backend's full-replace (not merge) PUT
 * semantics — clears that secret on update. There is no way to "leave it
 * as-is"; the UI must (and does) warn about this before submit.
 */
export function llmProviderFormToRequest(
  data: LlmProviderFormData,
): LlmProviderRequest {
  return {
    name: data.name.trim(),
    provider: data.provider,
    base_url: data.base_url.trim(),
    model: data.model.trim(),
    ...(data.api_key ? { api_key: data.api_key } : {}),
  };
}
