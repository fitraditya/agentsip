import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Spinner } from "@/components/ui/spinner";
import { useTranslations } from "@/i18n";
import { AlertIcon } from "@/icons";
import { ApiError, sttProvidersApi, type SttProvider } from "@/lib/api";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import SttProviderForm from "../form";
import {
  sttProviderFormToRequest,
  sttProviderToFormDefaults,
  type SttProviderFormData,
} from "../schema";

export default function SttProviderEditPage() {
  const { id } = useParams<{ id: string }>();
  const t = useTranslations("sttProviders.edit");
  const tc = useTranslations("common");
  const navigate = useNavigate();

  const [provider, setProvider] = useState<SttProvider | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    setLoadError(null);
    sttProvidersApi
      .get(id)
      .then(setProvider)
      .catch((err: unknown) => {
        setLoadError(
          err instanceof ApiError ? err.message : t("errors.loadFailed"),
        );
      })
      .finally(() => setLoading(false));
  }, [id, t]);

  const handleSubmit = async (data: SttProviderFormData) => {
    if (!id) return;
    setSubmitting(true);
    setServerError(null);
    try {
      await sttProvidersApi.update(id, sttProviderFormToRequest(data));
      void navigate("/stt-providers");
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
        title="Edit STT Provider | agentsip"
        description="Update a reusable speech-to-text provider."
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
        <SttProviderForm
          mode="edit"
          defaultValues={sttProviderToFormDefaults(provider)}
          apiKeyConfigured={provider.api_key_configured}
          serverError={serverError}
          submitting={submitting}
          onSubmit={(data) => void handleSubmit(data)}
          onCancel={() => void navigate("/stt-providers")}
        />
      ) : null}
    </>
  );
}
