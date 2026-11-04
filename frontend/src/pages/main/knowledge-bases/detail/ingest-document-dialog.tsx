import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useTranslations } from "@/i18n";
import { AlertIcon, FileIcon, TrashBinIcon, UploadIcon } from "@/icons";
import {
  ApiError,
  knowledgeBasesApi,
  type KnowledgeBaseDocument,
} from "@/lib/api";
import { cn } from "@/utils";
import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useState } from "react";
import { useDropzone } from "react-dropzone";
import { useForm } from "react-hook-form";
import { ingestDocumentSchema, type IngestDocumentFormData } from "./schema";

interface IngestDocumentDialogProps {
  knowledgeBaseId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onIngested: (document: KnowledgeBaseDocument) => void;
}

const MAX_UPLOAD_BYTES = 20 * 1024 * 1024;
const ACCEPTED_EXTENSIONS = [".pdf", ".txt", ".md"];

export default function IngestDocumentDialog({
  knowledgeBaseId,
  open,
  onOpenChange,
  onIngested,
}: IngestDocumentDialogProps) {
  const t = useTranslations("knowledgeBases.ingestDialog");
  const [mode, setMode] = useState<"paste" | "upload">("paste");

  useEffect(() => {
    if (!open) setMode("paste");
  }, [open]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>

        <div className="mb-1 flex gap-1 rounded-lg bg-gray-100 p-1 dark:bg-white/5">
          <button
            type="button"
            onClick={() => setMode("paste")}
            className={cn(
              "flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
              mode === "paste"
                ? "bg-white text-gray-800 shadow-theme-xs dark:bg-gray-800 dark:text-white/90"
                : "text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300",
            )}
          >
            {t("pasteTab")}
          </button>
          <button
            type="button"
            onClick={() => setMode("upload")}
            className={cn(
              "flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
              mode === "upload"
                ? "bg-white text-gray-800 shadow-theme-xs dark:bg-gray-800 dark:text-white/90"
                : "text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300",
            )}
          >
            {t("uploadTab")}
          </button>
        </div>

        {mode === "paste" ? (
          <PasteTextForm
            knowledgeBaseId={knowledgeBaseId}
            onCancel={() => onOpenChange(false)}
            onIngested={onIngested}
          />
        ) : (
          <UploadFileForm
            knowledgeBaseId={knowledgeBaseId}
            onCancel={() => onOpenChange(false)}
            onIngested={onIngested}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

interface FormProps {
  knowledgeBaseId: string;
  onCancel: () => void;
  onIngested: (document: KnowledgeBaseDocument) => void;
}

function PasteTextForm({ knowledgeBaseId, onCancel, onIngested }: FormProps) {
  const t = useTranslations("knowledgeBases.ingestDialog");
  const [formError, setFormError] = useState<string | null>(null);
  const [resultNotice, setResultNotice] = useState<{
    title: string;
    description: string;
  } | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<IngestDocumentFormData>({
    resolver: zodResolver(ingestDocumentSchema),
    defaultValues: { sourceUri: "", text: "" },
    mode: "onTouched",
  });

  const onSubmit = async (data: IngestDocumentFormData) => {
    setFormError(null);
    setResultNotice(null);
    try {
      // NOTE: a 201 here can still carry status "FAILED" with a
      // failure_reason — that's not a transport error, so check the
      // returned document's status explicitly before treating this as
      // a success.
      const document = await knowledgeBasesApi.ingestDocument(
        knowledgeBaseId,
        { source_uri: data.sourceUri, text: data.text },
      );

      if (document.status === "FAILED") {
        setResultNotice({
          title: t("failedTitle"),
          description:
            document.failure_reason ?? t("failedDescriptionFallback"),
        });
        return;
      }

      onIngested(document);
    } catch (err) {
      setFormError(
        err instanceof ApiError
          ? err.message
          : "Something went wrong. Please try again.",
      );
    }
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-5" noValidate>
      {formError && (
        <Alert variant="destructive">
          <AlertIcon />
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      )}

      {resultNotice && (
        <Alert variant="destructive">
          <AlertIcon />
          <AlertTitle>{resultNotice.title}</AlertTitle>
          <AlertDescription>{resultNotice.description}</AlertDescription>
        </Alert>
      )}

      <Field data-invalid={Boolean(errors.sourceUri)}>
        <FieldLabel>{t("sourceUri")}</FieldLabel>
        <Input
          {...register("sourceUri")}
          placeholder={t("sourceUriPlaceholder")}
          aria-invalid={errors.sourceUri ? "true" : undefined}
        />
        <FieldError errors={[errors.sourceUri]} />
      </Field>

      <Field data-invalid={Boolean(errors.text)}>
        <FieldLabel>{t("text")}</FieldLabel>
        <Textarea
          {...register("text")}
          rows={10}
          placeholder={t("textPlaceholder")}
          aria-invalid={errors.text ? "true" : undefined}
        />
        <FieldError errors={[errors.text]} />
      </Field>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("cancel")}
        </Button>
        <Button type="submit" disabled={isSubmitting}>
          {isSubmitting ? t("submitting") : t("submit")}
        </Button>
      </DialogFooter>
    </form>
  );
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function UploadFileForm({ knowledgeBaseId, onCancel, onIngested }: FormProps) {
  const t = useTranslations("knowledgeBases.ingestDialog");
  const [file, setFile] = useState<File | null>(null);
  const [sourceUri, setSourceUri] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [resultNotice, setResultNotice] = useState<{
    title: string;
    description: string;
  } | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const { getRootProps, getInputProps, isDragActive, isDragReject } =
    useDropzone({
      multiple: false,
      maxSize: MAX_UPLOAD_BYTES,
      accept: {
        "application/pdf": [".pdf"],
        "text/plain": [".txt"],
        "text/markdown": [".md"],
      },
      onDropAccepted: (accepted) => {
        setFormError(null);
        setFile(accepted[0] ?? null);
      },
      onDropRejected: (rejections) => {
        const reason = rejections[0]?.errors[0]?.code;
        setFormError(
          reason === "file-too-large"
            ? t("fileTooLarge")
            : t("unsupportedFileType"),
        );
      },
    });

  const onSubmit = async () => {
    if (!file) {
      setFormError(t("fileRequired"));
      return;
    }
    setFormError(null);
    setResultNotice(null);
    setSubmitting(true);
    try {
      const document = await knowledgeBasesApi.uploadDocument(
        knowledgeBaseId,
        file,
        sourceUri.trim() || undefined,
      );

      if (document.status === "FAILED") {
        setResultNotice({
          title: t("failedTitle"),
          description:
            document.failure_reason ?? t("failedDescriptionFallback"),
        });
        return;
      }

      onIngested(document);
    } catch (err) {
      setFormError(
        err instanceof ApiError
          ? err.message
          : "Something went wrong. Please try again.",
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-5">
      {formError && (
        <Alert variant="destructive">
          <AlertIcon />
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      )}

      {resultNotice && (
        <Alert variant="destructive">
          <AlertIcon />
          <AlertTitle>{resultNotice.title}</AlertTitle>
          <AlertDescription>{resultNotice.description}</AlertDescription>
        </Alert>
      )}

      {!file ? (
        <div
          {...getRootProps()}
          className={cn(
            "flex cursor-pointer flex-col items-center justify-center rounded-xl border border-dashed p-7 text-center transition-colors",
            isDragReject
              ? "border-red-500 bg-red-50/60 dark:bg-red-950/20"
              : isDragActive
                ? "border-brand-500 bg-brand-50/60 dark:bg-brand-950/20"
                : "border-gray-300 bg-gray-50 hover:border-brand-500 dark:border-gray-700 dark:bg-gray-900",
          )}
        >
          <input {...getInputProps()} accept={ACCEPTED_EXTENSIONS.join(",")} />
          <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-gray-200 text-gray-700 dark:bg-gray-800 dark:text-gray-300">
            <UploadIcon className="size-5" />
          </div>
          <p className="mb-1 text-sm font-medium text-gray-800 dark:text-white/90">
            {isDragActive ? t("dropzoneTitleActive") : t("dropzoneTitleIdle")}
          </p>
          <p className="mb-3 text-xs text-gray-500 dark:text-gray-400">
            {t("dropzoneSubtitle")}
          </p>
          <span className="text-sm font-medium text-brand-500 underline hover:text-brand-600">
            {t("dropzoneBrowse")}
          </span>
        </div>
      ) : (
        <div className="flex items-center gap-3 rounded-xl border border-gray-200 p-4 dark:border-gray-800">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-500 dark:bg-white/5 dark:text-gray-400">
            <FileIcon className="size-5" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium text-gray-800 dark:text-white/90">
              {file.name}
            </p>
            <p className="text-xs text-gray-500 dark:text-gray-400">
              {formatFileSize(file.size)}
            </p>
          </div>
          <button
            type="button"
            onClick={() => setFile(null)}
            aria-label={t("removeFile")}
            className="flex size-8 shrink-0 items-center justify-center rounded-full text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-gray-500 dark:hover:bg-white/5 dark:hover:text-gray-300"
          >
            <TrashBinIcon className="size-4" />
          </button>
        </div>
      )}

      <Field>
        <FieldLabel>{t("sourceUri")}</FieldLabel>
        <Input
          value={sourceUri}
          onChange={(e) => setSourceUri(e.target.value)}
          placeholder={file?.name ?? t("sourceUriPlaceholder")}
        />
      </Field>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("cancel")}
        </Button>
        <Button type="button" onClick={() => void onSubmit()} disabled={submitting}>
          {submitting ? t("uploadSubmitting") : t("uploadSubmit")}
        </Button>
      </DialogFooter>
    </div>
  );
}
