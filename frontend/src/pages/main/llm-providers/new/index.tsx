import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { useTranslations } from "@/i18n";
import { ApiError, llmProvidersApi } from "@/lib/api";
import { useState } from "react";
import { useNavigate } from "react-router";
import LlmProviderForm from "../form";
import {
  emptyLlmProviderFormDefaults,
  llmProviderFormToRequest,
  type LlmProviderFormData,
} from "../schema";

export default function LlmProviderNewPage() {
  const t = useTranslations("llmProviders.new");
  const navigate = useNavigate();

  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (data: LlmProviderFormData) => {
    setSubmitting(true);
    setServerError(null);
    try {
      await llmProvidersApi.create(llmProviderFormToRequest(data));
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
        title="New LLM Provider | agentsip"
        description="Create a reusable language model provider."
      />
      <PageBreadcrumb pageTitle={t("title")} />

      <LlmProviderForm
        mode="create"
        defaultValues={emptyLlmProviderFormDefaults()}
        serverError={serverError}
        submitting={submitting}
        onSubmit={(data) => void handleSubmit(data)}
        onCancel={() => void navigate("/llm-providers")}
      />
    </>
  );
}
