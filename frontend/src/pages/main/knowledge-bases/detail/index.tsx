import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useTranslations } from "@/i18n";
import { AlertIcon, ChevronLeftIcon, PlusIcon } from "@/icons";
import {
  ApiError,
  knowledgeBasesApi,
  type DocumentStatus,
  type KnowledgeBase,
  type KnowledgeBaseDocument,
} from "@/lib/api";
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import IngestDocumentDialog from "./ingest-document-dialog";

const STATUS_BADGE_COLOR: Record<
  DocumentStatus,
  "success" | "warning" | "error"
> = {
  READY: "success",
  PROCESSING: "warning",
  PENDING: "warning",
  FAILED: "error",
};

export default function KnowledgeBaseDetailPage() {
  const { id } = useParams<{ id: string }>();
  const t = useTranslations("knowledgeBases.detail");
  const td = useTranslations("knowledgeBases.documentStatus");
  const tCreate = useTranslations("knowledgeBases.createDialog");
  const tc = useTranslations("common");

  const [knowledgeBase, setKnowledgeBase] = useState<KnowledgeBase | null>(
    null,
  );
  const [documents, setDocuments] = useState<KnowledgeBaseDocument[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [ingestOpen, setIngestOpen] = useState(false);

  const load = async () => {
    if (!id) return;
    setLoading(true);
    setError(null);
    try {
      const [kb, docs] = await Promise.all([
        knowledgeBasesApi.get(id),
        knowledgeBasesApi.listDocuments(id),
      ]);
      setKnowledgeBase(kb);
      setDocuments(docs);
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : t("errors.loadFailed"),
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [id]);

  return (
    <>
      <PageMeta
        title="Knowledge Base | AgentSIP"
        description="View a knowledge base's configuration and documents."
      />

      <Link
        to="/knowledge-bases"
        className="mb-4 inline-flex items-center gap-1 text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
      >
        <ChevronLeftIcon className="rtl:rotate-180" />
        {t("backLink")}
      </Link>

      <PageBreadcrumb pageTitle={knowledgeBase?.name ?? t("backLink")} />

      {error && (
        <Alert variant="destructive" className="mb-6">
          <AlertIcon />
          <AlertTitle>{t("errors.loadFailed")}</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {loading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          {tc("loading")}
        </p>
      ) : knowledgeBase ? (
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>{t("configTitle")}</CardTitle>
            </CardHeader>
            <CardContent className="grid grid-cols-1 gap-5 sm:grid-cols-2">
              <div>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  {t("embeddingProvider")}
                </p>
                <p className="mt-1 text-sm font-medium text-gray-800 dark:text-white/90">
                  {knowledgeBase.embedding.provider}
                </p>
              </div>
              <div>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  {t("embeddingModel")}
                </p>
                <p className="mt-1 text-sm font-medium text-gray-800 dark:text-white/90">
                  {knowledgeBase.embedding.model}
                </p>
              </div>
              <div>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  {t("embeddingBaseUrl")}
                </p>
                <p className="mt-1 text-sm font-medium break-all text-gray-800 dark:text-white/90">
                  {knowledgeBase.embedding.base_url || "—"}
                </p>
              </div>
              <div>
                <p className="mb-1 text-xs text-gray-500 dark:text-gray-400">
                  {tCreate("apiKey")}
                </p>
                <Badge
                  color={
                    knowledgeBase.embedding.api_key_configured
                      ? "success"
                      : "gray"
                  }
                >
                  {knowledgeBase.embedding.api_key_configured
                    ? t("apiKeyConfigured")
                    : t("apiKeyNotConfigured")}
                </Badge>
              </div>
              <div>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  {t("vectorStoreTitle")}
                </p>
                <Badge
                  className="mt-1"
                  color={
                    knowledgeBase.vector_store.custom_dsn_set
                      ? "info"
                      : "gray"
                  }
                >
                  {knowledgeBase.vector_store.custom_dsn_set
                    ? `${knowledgeBase.vector_store.provider} — ${t("vectorStoreCustomDsn")}`
                    : t("vectorStorePlatformManaged")}
                </Badge>
              </div>
            </CardContent>
          </Card>

          <Card className="gap-0 overflow-hidden p-0">
            <CardHeader className="flex-row items-center justify-between gap-3 p-5 sm:p-6">
              <CardTitle className="mb-0">{t("documentsTitle")}</CardTitle>
              <Button size="sm" onClick={() => setIngestOpen(true)}>
                <PlusIcon />
                {t("ingestButton")}
              </Button>
            </CardHeader>
            <CardContent className="border-t border-gray-100 p-0 dark:border-gray-800">
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader className="border-b border-gray-100 dark:border-gray-800">
                    <TableRow>
                      <TableCell
                        isHeader
                        className="px-5 py-3 text-start text-sm font-normal text-gray-500 sm:px-6 dark:text-gray-400"
                      >
                        {t("documentsTable.sourceUri")}
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-4 py-3 text-start text-sm font-normal text-gray-500 dark:text-gray-400"
                      >
                        {t("documentsTable.status")}
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-4 py-3 text-start text-sm font-normal text-gray-500 sm:px-6 dark:text-gray-400"
                      >
                        {t("documentsTable.failureReason")}
                      </TableCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody className="divide-y divide-gray-100 dark:divide-gray-800">
                    {documents.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={3}
                          className="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400"
                        >
                          {t("documentsEmpty")}
                        </TableCell>
                      </TableRow>
                    ) : (
                      documents.map((doc) => (
                        <TableRow key={doc.id}>
                          <TableCell className="max-w-sm px-5 py-4 text-start text-sm break-all text-gray-800 sm:px-6 dark:text-white/90">
                            {doc.source_uri}
                          </TableCell>
                          <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap">
                            <Badge
                              size="sm"
                              color={STATUS_BADGE_COLOR[doc.status]}
                            >
                              {td(doc.status)}
                            </Badge>
                          </TableCell>
                          <TableCell className="px-4 py-4 text-start text-sm text-gray-500 sm:px-6 dark:text-gray-400">
                            {doc.status === "FAILED" && doc.failure_reason
                              ? doc.failure_reason
                              : "—"}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        </div>
      ) : null}

      {id && (
        <IngestDocumentDialog
          knowledgeBaseId={id}
          open={ingestOpen}
          onOpenChange={setIngestOpen}
          onIngested={() => {
            setIngestOpen(false);
            void load();
          }}
        />
      )}
    </>
  );
}
