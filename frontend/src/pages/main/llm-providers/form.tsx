import { ComponentCard } from "@/components/shared/component-card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import { useTranslations } from "@/i18n";
import { AlertIcon } from "@/icons";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import {
  llmProviderFormSchema,
  llmProviderOptions,
  type LlmProviderFormData,
} from "./schema";

interface LlmProviderFormProps {
  mode: "create" | "edit";
  defaultValues: LlmProviderFormData;
  apiKeyConfigured?: boolean;
  serverError: string | null;
  submitting: boolean;
  onSubmit: (data: LlmProviderFormData) => void;
  onCancel: () => void;
}

export default function LlmProviderForm({
  mode,
  defaultValues,
  apiKeyConfigured,
  serverError,
  submitting,
  onSubmit,
  onCancel,
}: LlmProviderFormProps) {
  const t = useTranslations("llmProviders.form");
  const tn = useTranslations("llmProviders.new");
  const te = useTranslations("llmProviders.edit");

  const {
    register,
    handleSubmit,
    control,
    formState: { errors },
  } = useForm<LlmProviderFormData>({
    resolver: zodResolver(llmProviderFormSchema),
    defaultValues,
    mode: "onTouched",
  });

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-6" noValidate>
      {serverError && (
        <Alert variant="destructive">
          <AlertIcon />
          <AlertTitle>{t("saveErrorTitle")}</AlertTitle>
          <AlertDescription>{serverError}</AlertDescription>
        </Alert>
      )}

      {mode === "edit" && (
        <Alert variant="warning">
          <AlertIcon />
          <AlertTitle>{t("fullReplaceWarningTitle")}</AlertTitle>
          <AlertDescription>{t("fullReplaceWarningBody")}</AlertDescription>
        </Alert>
      )}

      <ComponentCard title={t("sectionTitle")}>
        <Field data-invalid={Boolean(errors.name)} className="gap-1.5">
          <FieldLabel>{t("name")}</FieldLabel>
          <Input
            {...register("name")}
            placeholder={t("namePlaceholder")}
            aria-invalid={errors.name ? "true" : undefined}
          />
          <FieldError errors={[errors.name]} />
        </Field>

        <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
          <Field className="gap-1.5">
            <FieldLabel>{t("provider")}</FieldLabel>
            <Controller
              control={control}
              name="provider"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  items={llmProviderOptions}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t("providerPlaceholder")} />
                  </SelectTrigger>
                  <SelectContent>
                    {llmProviderOptions.map((p) => (
                      <SelectItem key={p.value} value={p.value}>
                        {p.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </Field>

          <Field data-invalid={Boolean(errors.model)} className="gap-1.5">
            <FieldLabel>{t("model")}</FieldLabel>
            <Input
              {...register("model")}
              placeholder={t("modelPlaceholder")}
              aria-invalid={errors.model ? "true" : undefined}
            />
            <FieldError errors={[errors.model]} />
          </Field>
        </div>

        <Field data-invalid={Boolean(errors.base_url)} className="gap-1.5">
          <FieldLabel>{t("baseUrl")}</FieldLabel>
          <Input
            {...register("base_url")}
            placeholder={t("baseUrlPlaceholder")}
            aria-invalid={errors.base_url ? "true" : undefined}
          />
          <FieldError errors={[errors.base_url]} />
        </Field>

        <Field className="gap-1.5">
          <FieldLabel>{t("apiKey")}</FieldLabel>
          <Input
            {...register("api_key")}
            type="password"
            autoComplete="new-password"
            placeholder={
              mode === "edit"
                ? apiKeyConfigured
                  ? t("apiKeyConfigured")
                  : t("apiKeyNotSet")
                : t("apiKeyOptional")
            }
          />
        </Field>
      </ComponentCard>

      <div className="flex justify-end gap-3">
        <Button variant="outline" type="button" onClick={onCancel}>
          {mode === "create" ? tn("cancel") : te("cancel")}
        </Button>
        <Button type="submit" disabled={submitting}>
          {mode === "create"
            ? submitting
              ? tn("submitting")
              : tn("submit")
            : submitting
              ? te("submitting")
              : te("submit")}
        </Button>
      </div>
    </form>
  );
}
