import { ComponentCard } from "@/components/shared/component-card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { AlertIcon, XIcon } from "@/icons";
import type {
  KnowledgeBase,
  LlmProvider,
  SttProvider,
  TtsProvider,
} from "@/lib/api";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { Link } from "react-router";
import { trunkFormSchema, type TrunkFormData } from "./schema";

export interface SecretConfiguredFlags {
  digest_configured: boolean;
}

interface TrunkFormProps {
  mode: "create" | "edit";
  defaultValues: TrunkFormData;
  configured?: SecretConfiguredFlags;
  knowledgeBases: KnowledgeBase[];
  sttProviders: SttProvider[];
  llmProviders: LlmProvider[];
  ttsProviders: TtsProvider[];
  serverError: string | null;
  submitting: boolean;
  onSubmit: (data: TrunkFormData) => void;
  onCancel: () => void;
}

function secretPlaceholder(configured: boolean | undefined): string {
  return configured ? "configured — leave blank to clear" : "not set";
}

export default function TrunkForm({
  mode,
  defaultValues,
  configured,
  knowledgeBases,
  sttProviders,
  llmProviders,
  ttsProviders,
  serverError,
  submitting,
  onSubmit,
  onCancel,
}: TrunkFormProps) {
  const {
    register,
    handleSubmit,
    control,
    watch,
    formState: { errors },
  } = useForm<TrunkFormData>({
    resolver: zodResolver(trunkFormSchema),
    defaultValues,
    mode: "onTouched",
  });

  const answerMode = watch("answer_mode");

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-6" noValidate>
      {serverError && (
        <Alert variant="destructive">
          <AlertIcon />
          <AlertTitle>Couldn't save trunk</AlertTitle>
          <AlertDescription>{serverError}</AlertDescription>
        </Alert>
      )}

      {mode === "edit" && (
        <Alert variant="warning">
          <AlertIcon />
          <AlertTitle>Saving resends the full configuration</AlertTitle>
          <AlertDescription>
            Editing this trunk resends its full configuration — this isn't a
            partial update. The digest password left blank below will be
            cleared; re-enter it only if you want to change it. STT/LLM/TTS
            providers are separate resources now, so their secrets aren't
            affected here — edit them from their own provider pages.
          </AlertDescription>
        </Alert>
      )}

      <ComponentCard title="General">
        <Field data-invalid={Boolean(errors.name)} className="gap-1.5">
          <FieldLabel>Name</FieldLabel>
          <Input
            {...register("name")}
            placeholder="e.g. Primary SIP trunk"
            aria-invalid={errors.name ? "true" : undefined}
          />
          <FieldError errors={[errors.name]} />
        </Field>

        <Field className="gap-1.5">
          <FieldLabel>Knowledge bases</FieldLabel>
          <Controller
            control={control}
            name="knowledge_base_ids"
            render={({ field }) => (
              <Select
                multiple
                value={field.value}
                onValueChange={(val) => field.onChange(val as string[])}
              >
                <SelectTrigger
                  render={<div />}
                  nativeButton={false}
                  className="h-auto min-h-11 cursor-pointer py-1.5 ps-3 pe-3.5"
                >
                  <SelectValue placeholder="No knowledge bases attached">
                    {(values: string[]) => {
                      if (!values || values.length === 0) return null;
                      return (
                        <div className="flex flex-wrap items-center gap-2">
                          {values.map((id) => {
                            const kb = knowledgeBases.find(
                              (k) => k.id === id,
                            );
                            return (
                              <span
                                key={id}
                                className="inline-flex items-center gap-1.5 rounded-full border border-gray-200 bg-gray-100 py-1 ps-2.5 pe-1.5 text-sm font-normal text-gray-800 dark:border-gray-800 dark:bg-white/5 dark:text-white/90"
                              >
                                <span>{kb?.name ?? id}</span>
                                <button
                                  type="button"
                                  aria-label={`Remove ${kb?.name ?? id}`}
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    field.onChange(
                                      field.value.filter((v) => v !== id),
                                    );
                                  }}
                                  onPointerDown={(e) => e.stopPropagation()}
                                  className="inline-flex size-3.5 cursor-pointer items-center justify-center rounded-full text-gray-400 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
                                >
                                  <XIcon className="size-3" />
                                </button>
                              </span>
                            );
                          })}
                        </div>
                      );
                    }}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {knowledgeBases.length === 0 ? (
                    <div className="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">
                      No knowledge bases exist yet.
                    </div>
                  ) : (
                    knowledgeBases.map((kb) => (
                      <SelectItem key={kb.id} value={kb.id}>
                        {kb.name}
                      </SelectItem>
                    ))
                  )}
                </SelectContent>
              </Select>
            )}
          />
          <FieldDescription>
            Optional. Attach knowledge bases this trunk's agent can draw on
            when answering calls.
          </FieldDescription>
        </Field>

        <Field className="gap-1.5">
          <FieldLabel>System prompt</FieldLabel>
          <Textarea
            {...register("system_prompt")}
            rows={4}
            placeholder="You are a helpful voice assistant for..."
            aria-invalid={errors.system_prompt ? "true" : undefined}
          />
          <FieldError errors={[errors.system_prompt]} />
        </Field>

        <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
          <Field
            data-invalid={Boolean(errors.max_concurrent_calls)}
            className="gap-1.5"
          >
            <FieldLabel>Max concurrent calls</FieldLabel>
            <Input
              {...register("max_concurrent_calls")}
              type="number"
              min={0}
              placeholder="Defaults to 1"
            />
            <FieldError errors={[errors.max_concurrent_calls]} />
          </Field>

          <Field className="gap-1.5">
            <FieldLabel>Answer mode</FieldLabel>
            <Controller
              control={control}
              name="answer_mode"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  items={[
                    { value: "auto", label: "Auto" },
                    { value: "webhook", label: "Webhook" },
                  ]}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select answer mode" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">Auto</SelectItem>
                    <SelectItem value="webhook">Webhook</SelectItem>
                  </SelectContent>
                </Select>
              )}
            />
          </Field>
        </div>

        {answerMode === "auto" ? (
          <Field
            data-invalid={Boolean(errors.ring_delay_seconds)}
            className="gap-1.5 sm:max-w-60"
          >
            <FieldLabel>Ring delay (seconds)</FieldLabel>
            <Input
              {...register("ring_delay_seconds")}
              type="number"
              min={0}
              placeholder="Defaults server-side"
            />
            <FieldError errors={[errors.ring_delay_seconds]} />
          </Field>
        ) : (
          <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
            <Field
              data-invalid={Boolean(errors.webhook_url)}
              className="gap-1.5"
            >
              <FieldLabel>Webhook URL</FieldLabel>
              <Input
                {...register("webhook_url")}
                placeholder="https://example.com/webhook"
                aria-invalid={errors.webhook_url ? "true" : undefined}
              />
              <FieldError errors={[errors.webhook_url]} />
            </Field>
            <Field
              data-invalid={Boolean(errors.webhook_timeout_seconds)}
              className="gap-1.5"
            >
              <FieldLabel>Webhook timeout (seconds)</FieldLabel>
              <Input
                {...register("webhook_timeout_seconds")}
                type="number"
                min={0}
                placeholder="Defaults to 30"
              />
              <FieldError errors={[errors.webhook_timeout_seconds]} />
            </Field>
          </div>
        )}
      </ComponentCard>

      <ComponentCard title="SIP authentication">
        <Field data-invalid={Boolean(errors.allowed_ips)} className="gap-1.5">
          <FieldLabel>Allowed IPs</FieldLabel>
          <Input
            {...register("allowed_ips")}
            placeholder="203.0.113.10, 203.0.113.11"
          />
          <FieldDescription>
            Comma-separated list of IPs/CIDRs allowed to connect.
          </FieldDescription>
          <FieldError errors={[errors.allowed_ips]} />
        </Field>

        <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
          <Field
            data-invalid={Boolean(errors.digest_username)}
            className="gap-1.5"
          >
            <FieldLabel>Digest username</FieldLabel>
            <Input {...register("digest_username")} />
            <FieldError errors={[errors.digest_username]} />
          </Field>
          <Field className="gap-1.5">
            <FieldLabel>Digest password</FieldLabel>
            <Input
              {...register("digest_password")}
              type="password"
              autoComplete="new-password"
              placeholder={
                mode === "edit"
                  ? secretPlaceholder(configured?.digest_configured)
                  : "Optional"
              }
            />
          </Field>
        </div>
      </ComponentCard>

      <ComponentCard
        title="Providers"
        description="STT, LLM, and TTS are configured once as standalone provider resources and referenced here by name."
      >
        <Field data-invalid={Boolean(errors.stt_provider_id)} className="gap-1.5">
          <FieldLabel>Speech-to-text provider</FieldLabel>
          {sttProviders.length === 0 ? (
            <FieldDescription>
              No STT providers yet —{" "}
              <Link to="/stt-providers/new">create one first</Link>.
            </FieldDescription>
          ) : (
            <Controller
              control={control}
              name="stt_provider_id"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  items={sttProviders.map((p) => ({
                    value: p.id,
                    label: p.name,
                  }))}
                >
                  <SelectTrigger
                    aria-invalid={
                      errors.stt_provider_id ? "true" : undefined
                    }
                  >
                    <SelectValue placeholder="Select STT provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {sttProviders.map((p) => (
                      <SelectItem key={p.id} value={p.id}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          )}
          <FieldError errors={[errors.stt_provider_id]} />
        </Field>

        <Field data-invalid={Boolean(errors.llm_provider_id)} className="gap-1.5">
          <FieldLabel>Language model provider</FieldLabel>
          {llmProviders.length === 0 ? (
            <FieldDescription>
              No LLM providers yet —{" "}
              <Link to="/llm-providers/new">create one first</Link>.
            </FieldDescription>
          ) : (
            <Controller
              control={control}
              name="llm_provider_id"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  items={llmProviders.map((p) => ({
                    value: p.id,
                    label: p.name,
                  }))}
                >
                  <SelectTrigger
                    aria-invalid={
                      errors.llm_provider_id ? "true" : undefined
                    }
                  >
                    <SelectValue placeholder="Select LLM provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {llmProviders.map((p) => (
                      <SelectItem key={p.id} value={p.id}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          )}
          <FieldError errors={[errors.llm_provider_id]} />
        </Field>

        <Field data-invalid={Boolean(errors.tts_provider_id)} className="gap-1.5">
          <FieldLabel>Text-to-speech provider</FieldLabel>
          {ttsProviders.length === 0 ? (
            <FieldDescription>
              No TTS providers yet —{" "}
              <Link to="/tts-providers/new">create one first</Link>.
            </FieldDescription>
          ) : (
            <Controller
              control={control}
              name="tts_provider_id"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  items={ttsProviders.map((p) => ({
                    value: p.id,
                    label: p.name,
                  }))}
                >
                  <SelectTrigger
                    aria-invalid={
                      errors.tts_provider_id ? "true" : undefined
                    }
                  >
                    <SelectValue placeholder="Select TTS provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {ttsProviders.map((p) => (
                      <SelectItem key={p.id} value={p.id}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          )}
          <FieldError errors={[errors.tts_provider_id]} />
        </Field>
      </ComponentCard>

      <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
        <Button variant="outline" type="button" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" disabled={submitting}>
          {submitting
            ? "Saving..."
            : mode === "create"
              ? "Create trunk"
              : "Save changes"}
        </Button>
      </div>
    </form>
  );
}
