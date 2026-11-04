import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { useTranslations } from "@/i18n";
import { ApiError, ttsProvidersApi } from "@/lib/api";
import { useState } from "react";
import { useNavigate } from "react-router";
import TtsProviderForm from "../form";
import {
  emptyTtsProviderFormDefaults,
  ttsProviderFormToRequest,
  type TtsProviderFormData,
} from "../schema";

export default function TtsProviderNewPage() {
  const t = useTranslations("ttsProviders.new");
  const navigate = useNavigate();

  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (data: TtsProviderFormData) => {
    setSubmitting(true);
    setServerError(null);
    try {
      await ttsProvidersApi.create(ttsProviderFormToRequest(data));
      void navigate("/tts-providers");
    } catch (err) {
      setServerError(
        err instanceof ApiError ? err.message : t("genericError"),
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <PageMeta
        title="New TTS Provider | agentsip"
        description="Create a reusable text-to-speech provider."
      />
      <PageBreadcrumb pageTitle={t("title")} />

      <TtsProviderForm
        mode="create"
        defaultValues={emptyTtsProviderFormDefaults()}
        serverError={serverError}
        submitting={submitting}
        onSubmit={(data) => void handleSubmit(data)}
        onCancel={() => void navigate("/tts-providers")}
      />
    </>
  );
}
