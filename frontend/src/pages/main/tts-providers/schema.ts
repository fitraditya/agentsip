import type { ProviderKind, TtsProvider, TtsProviderRequest } from "@/lib/api";
import { z } from "zod";

/** TTS only supports ElevenLabs today — the type allows the full
 * `ProviderKind` union, but the UI only ever offers this one choice. */
export const ttsProviderOptions: { value: ProviderKind; label: string }[] = [
  { value: "elevenlabs", label: "ElevenLabs" },
];

const providerEnum = z.enum(["elevenlabs"]);

export const ttsProviderFormSchema = z.object({
  name: z.string().trim().min(1, "Name is required"),
  provider: providerEnum,
  api_key: z.string().optional(),
  voice_id: z.string().trim().min(1, "Voice ID is required"),
  model: z.string().trim().min(1, "Model is required"),
});

export type TtsProviderFormData = z.infer<typeof ttsProviderFormSchema>;

export function emptyTtsProviderFormDefaults(): TtsProviderFormData {
  return {
    name: "",
    provider: "elevenlabs",
    api_key: "",
    voice_id: "",
    model: "",
  };
}

/** Secrets never come back — the GET response never echoes `api_key`. */
export function ttsProviderToFormDefaults(
  provider: TtsProvider,
): TtsProviderFormData {
  return {
    name: provider.name,
    // The backend's ProviderKind is wider than this form's single-option
    // enum; narrowed here since "elevenlabs" is the only value the UI ever
    // writes back.
    provider: provider.provider as "elevenlabs",
    api_key: "",
    voice_id: provider.voice_id,
    model: provider.model,
  };
}

/**
 * Builds the full-replace `TtsProviderRequest` body. A blank `api_key` is
 * omitted here, which — per the backend's full-replace (not merge) PUT
 * semantics — clears that secret on update. There is no way to "leave it
 * as-is"; the UI must (and does) warn about this before submit.
 */
export function ttsProviderFormToRequest(
  data: TtsProviderFormData,
): TtsProviderRequest {
  return {
    name: data.name.trim(),
    provider: data.provider,
    voice_id: data.voice_id.trim(),
    model: data.model.trim(),
    ...(data.api_key ? { api_key: data.api_key } : {}),
  };
}
