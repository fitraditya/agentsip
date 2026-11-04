import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Spinner } from "@/components/ui/spinner";
import { useTranslations } from "@/i18n";
import { AlertIcon } from "@/icons";
import { ApiError, llmProvidersApi, type LlmProvider } from "@/lib/api";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import LlmProviderForm from "../form";
import {
  llmProviderFormToRequest,
  llmProviderToFormDefaults,
  type LlmProviderFormData,
} from "../schema";

export default function LlmProviderEditPage() {
  const { id } = useParams<{ id: string }>();
  const t = useTranslations("llmProviders.edit");
  const tc = useTranslations("common");
  const navigate = useNavigate();

  const [provider, setProvider] = useState<LlmProvider | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    setLoadError(null);
    llmProvidersApi
      .get(id)
      .then(setProvider)
      .catch((err: unknown) => {
        setLoadError(
          err instanceof ApiError ? err.message : t("errors.loadFailed"),
        );
      })
      .finally(() => setLoading(false));
  }, [id, t]);

  const handleSubmit = async (data: LlmProviderFormData) => {
    if (!id) return;
    setSubmitting(true);
    setServerError(null);
    try {
      await llmProvidersApi.update(id, llmProviderFormToRequest(data));
      void navigate("/llm-providers");
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
        title="Edit LLM Provider | agentsip"
        description="Update a reusable language model provider."
      />
      <PageBreadcrumb pageTitle={t("title")} />

      {loadError && (
        <Alert variant="destructive" className="mb-6">
          <AlertIcon />
          <AlertTitle>{t("errors.loadFailed")}</AlertTitle>
          <AlertDescription>{loadError}</AlertDescription>
        </Alert>
      )}

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-16 text-sm text-gray-500 dark:text-gray-400">
          <Spinner /> {tc("loading")}
        </div>
      ) : provider ? (
        <LlmProviderForm
          mode="edit"
          defaultValues={llmProviderToFormDefaults(provider)}
          apiKeyConfigured={provider.api_key_configured}
          serverError={serverError}
          submitting={submitting}
          onSubmit={(data) => void handleSubmit(data)}
          onCancel={() => void navigate("/llm-providers")}
        />
      ) : null}
    </>
  );
}
