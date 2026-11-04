import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { useTranslations } from "@/i18n";
import { ApiError, sttProvidersApi } from "@/lib/api";
import { useState } from "react";
import { useNavigate } from "react-router";
import SttProviderForm from "../form";
import {
  emptySttProviderFormDefaults,
  sttProviderFormToRequest,
  type SttProviderFormData,
} from "../schema";

export default function SttProviderNewPage() {
  const t = useTranslations("sttProviders.new");
  const navigate = useNavigate();

  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (data: SttProviderFormData) => {
    setSubmitting(true);
    setServerError(null);
    try {
      await sttProvidersApi.create(sttProviderFormToRequest(data));
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
        title="New STT Provider | agentsip"
        description="Create a reusable speech-to-text provider."
      />
      <PageBreadcrumb pageTitle={t("title")} />

      <SttProviderForm
        mode="create"
        defaultValues={emptySttProviderFormDefaults()}
        serverError={serverError}
        submitting={submitting}
        onSubmit={(data) => void handleSubmit(data)}
        onCancel={() => void navigate("/stt-providers")}
      />
    </>
  );
}
