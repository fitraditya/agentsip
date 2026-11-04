import type { ProviderKind, SttProvider, SttProviderRequest } from "@/lib/api";
import { z } from "zod";

/** STT only supports ElevenLabs today — the type allows the full
 * `ProviderKind` union, but the UI only ever offers this one choice. */
export const sttProviderOptions: { value: ProviderKind; label: string }[] = [
  { value: "elevenlabs", label: "ElevenLabs" },
];

const providerEnum = z.enum(["elevenlabs"]);

export const sttProviderFormSchema = z.object({
  name: z.string().trim().min(1, "Name is required"),
  provider: providerEnum,
  api_key: z.string().optional(),
  model: z.string().trim().min(1, "Model is required"),
});

export type SttProviderFormData = z.infer<typeof sttProviderFormSchema>;

export function emptySttProviderFormDefaults(): SttProviderFormData {
  return {
    name: "",
    provider: "elevenlabs",
    api_key: "",
    model: "",
  };
}

/** Secrets never come back — the GET response never echoes `api_key`. */
export function sttProviderToFormDefaults(
  provider: SttProvider,
): SttProviderFormData {
  return {
    name: provider.name,
    // The backend's ProviderKind is wider than this form's single-option
    // enum; narrowed here since "elevenlabs" is the only value the UI ever
    // writes back.
    provider: provider.provider as "elevenlabs",
    api_key: "",
    model: provider.model,
  };
}

/**
 * Builds the full-replace `SttProviderRequest` body. A blank `api_key` is
 * omitted here, which — per the backend's full-replace (not merge) PUT
 * semantics — clears that secret on update. There is no way to "leave it
 * as-is"; the UI must (and does) warn about this before submit.
 */
export function sttProviderFormToRequest(
  data: SttProviderFormData,
): SttProviderRequest {
  return {
    name: data.name.trim(),
    provider: data.provider,
    model: data.model.trim(),
    ...(data.api_key ? { api_key: data.api_key } : {}),
  };
}
