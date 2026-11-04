import { ComponentCard } from "@/components/shared/component-card";
import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useTranslations } from "@/i18n";
import { AlertIcon, ChevronLeftIcon } from "@/icons";
import { ApiError, knowledgeBasesApi } from "@/lib/api";
import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { Link, useNavigate } from "react-router";
import {
  createKnowledgeBaseSchema,
  type CreateKnowledgeBaseFormData,
} from "./schema";

export default function KnowledgeBaseNewPage() {
  const t = useTranslations("knowledgeBases.createDialog");
  const tList = useTranslations("knowledgeBases");
  const navigate = useNavigate();
  const [formError, setFormError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    control,
    watch,
    formState: { errors, isSubmitting },
  } = useForm<CreateKnowledgeBaseFormData>({
    resolver: zodResolver(createKnowledgeBaseSchema),
    defaultValues: {
      name: "",
      embeddingProvider: "openai_compatible",
      embeddingBaseUrl: "",
      embeddingApiKey: "",
      embeddingModel: "",
      useCustomVectorStore: false,
      vectorStoreDsn: "",
    },
    mode: "onTouched",
  });

  const useCustomVectorStore = watch("useCustomVectorStore");

  const onSubmit = async (data: CreateKnowledgeBaseFormData) => {
    setFormError(null);
    try {
      await knowledgeBasesApi.create({
        name: data.name,
        embedding: {
          provider: data.embeddingProvider,
          base_url: data.embeddingBaseUrl,
          api_key: data.embeddingApiKey,
          model: data.embeddingModel,
        },
        ...(data.useCustomVectorStore
          ? {
              vector_store: {
                provider: "pgvector" as const,
                dsn: data.vectorStoreDsn ?? "",
              },
            }
          : {}),
      });
      void navigate("/knowledge-bases");
    } catch (err) {
      setFormError(
        err instanceof ApiError
          ? err.message
          : "Something went wrong. Please try again.",
      );
    }
  };

  return (
    <div>
      <PageMeta
        title="New Knowledge Base | AgentSIP"
        description="Create a new knowledge base."
      />

      <Link
        to="/knowledge-bases"
        className="mb-4 inline-flex items-center gap-1 text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
      >
        <ChevronLeftIcon className="rtl:rotate-180" />
        {tList("title")}
      </Link>

      <PageBreadcrumb pageTitle={t("title")} />

      <form onSubmit={handleSubmit(onSubmit)} className="space-y-6" noValidate>
        {formError && (
          <Alert variant="destructive">
            <AlertIcon />
            <AlertDescription>{formError}</AlertDescription>
          </Alert>
        )}

        <ComponentCard title={t("generalSection")}>
          <Field data-invalid={Boolean(errors.name)}>
            <FieldLabel>{t("name")}</FieldLabel>
            <Input
              {...register("name")}
              placeholder={t("namePlaceholder")}
              aria-invalid={errors.name ? "true" : undefined}
            />
            <FieldError errors={[errors.name]} />
          </Field>
        </ComponentCard>

        <ComponentCard title={t("embeddingSection")}>
          <Field>
            <FieldLabel>{t("provider")}</FieldLabel>
            <Controller
              control={control}
              name="embeddingProvider"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  items={[
                    { value: "openai_compatible", label: "OpenAI-compatible" },
                  ]}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="openai_compatible">
                      OpenAI-compatible
                    </SelectItem>
                  </SelectContent>
                </Select>
              )}
            />
          </Field>

          <Field data-invalid={Boolean(errors.embeddingBaseUrl)}>
            <FieldLabel>{t("baseUrl")}</FieldLabel>
            <Input
              {...register("embeddingBaseUrl")}
              placeholder={t("baseUrlPlaceholder")}
              aria-invalid={errors.embeddingBaseUrl ? "true" : undefined}
            />
            <FieldError errors={[errors.embeddingBaseUrl]} />
          </Field>

          <Field data-invalid={Boolean(errors.embeddingApiKey)}>
            <FieldLabel>{t("apiKey")}</FieldLabel>
            <Input
              {...register("embeddingApiKey")}
              type="password"
              autoComplete="off"
              placeholder={t("apiKeyPlaceholder")}
              aria-invalid={errors.embeddingApiKey ? "true" : undefined}
            />
            <FieldError errors={[errors.embeddingApiKey]} />
          </Field>

          <Field data-invalid={Boolean(errors.embeddingModel)}>
            <FieldLabel>{t("model")}</FieldLabel>
            <Input
              {...register("embeddingModel")}
              placeholder={t("modelPlaceholder")}
              aria-invalid={errors.embeddingModel ? "true" : undefined}
            />
            <FieldError errors={[errors.embeddingModel]} />
          </Field>
        </ComponentCard>

        <ComponentCard title={t("vectorStoreSection")}>
          <Field
            orientation="horizontal"
            className="items-start justify-between gap-3"
          >
            <div className="flex flex-col gap-0.5">
              <FieldLabel htmlFor="useCustomVectorStore">
                {t("customVectorStore")}
              </FieldLabel>
              <p className="text-sm text-gray-500 dark:text-gray-400">
                {t("customVectorStoreDescription")}
              </p>
            </div>
            <Controller
              control={control}
              name="useCustomVectorStore"
              render={({ field }) => (
                <Switch
                  id="useCustomVectorStore"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
          </Field>

          {useCustomVectorStore && (
            <Field data-invalid={Boolean(errors.vectorStoreDsn)}>
              <FieldLabel>{t("dsn")}</FieldLabel>
              <Input
                {...register("vectorStoreDsn")}
                placeholder={t("dsnPlaceholder")}
                aria-invalid={errors.vectorStoreDsn ? "true" : undefined}
              />
              <FieldError errors={[errors.vectorStoreDsn]} />
            </Field>
          )}
        </ComponentCard>

        <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <Button
            variant="outline"
            render={<Link to="/knowledge-bases" />}
            nativeButton={false}
          >
            {t("cancel")}
          </Button>
          <Button type="submit" disabled={isSubmitting}>
            {isSubmitting ? t("submitting") : t("submit")}
          </Button>
        </div>
      </form>
    </div>
  );
}
