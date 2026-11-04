import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useTranslations } from "@/i18n";
import { AlertIcon, PencilIcon, PlusIcon, TrashBinIcon } from "@/icons";
import { ApiError, llmProvidersApi, type LlmProvider } from "@/lib/api";
import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router";

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

export default function LlmProvidersListPage() {
  const t = useTranslations("llmProviders");
  const tc = useTranslations("common");
  const navigate = useNavigate();

  const [providers, setProviders] = useState<LlmProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const loadProviders = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await llmProvidersApi.list();
      setProviders(data);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("errors.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void loadProviders();
  }, [loadProviders]);

  const handleDelete = async (provider: LlmProvider) => {
    const confirmed = window.confirm(
      t("deleteConfirm", { name: provider.name }),
    );
    if (!confirmed) return;

    setDeletingId(provider.id);
    setError(null);
    try {
      await llmProvidersApi.delete(provider.id);
      await loadProviders();
    } catch (err) {
      if (err instanceof ApiError && err.code === "CONFLICT") {
        setError(t("errors.deleteConflict"));
      } else {
        setError(err instanceof ApiError ? err.message : t("errors.deleteFailed"));
      }
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <>
      <PageMeta
        title="LLM Providers | agentsip"
        description="Manage reusable language model providers for your trunks."
      />
      <PageBreadcrumb pageTitle={t("title")} />

      {error && (
        <Alert variant="destructive" className="mb-6">
          <AlertIcon />
          <AlertTitle>{t("errors.loadFailed")}</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card className="gap-0 overflow-hidden p-0">
        <CardHeader className="flex-row items-center justify-between gap-3 p-5 sm:p-6">
          <CardTitle className="mb-0">{t("title")}</CardTitle>
          <Button size="sm" onClick={() => void navigate("/llm-providers/new")}>
            <PlusIcon />
            {t("createButton")}
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
                    {t("table.name")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-start text-sm font-normal text-gray-500 dark:text-gray-400"
                  >
                    {t("table.provider")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-start text-sm font-normal text-gray-500 dark:text-gray-400"
                  >
                    {t("table.model")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-start text-sm font-normal text-gray-500 dark:text-gray-400"
                  >
                    {t("table.baseUrl")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-start text-sm font-normal text-gray-500 dark:text-gray-400"
                  >
                    {t("table.createdAt")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-end text-sm font-normal text-gray-500 sm:px-6 dark:text-gray-400"
                  >
                    {t("table.actions")}
                  </TableCell>
                </TableRow>
              </TableHeader>

              <TableBody className="divide-y divide-gray-100 dark:divide-gray-800">
                {loading ? (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400"
                    >
                      {tc("loading")}
                    </TableCell>
                  </TableRow>
                ) : providers.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400"
                    >
                      {t("empty")}
                    </TableCell>
                  </TableRow>
                ) : (
                  providers.map((provider) => (
                    <TableRow key={provider.id}>
                      <TableCell className="px-5 py-4 text-start text-sm font-medium whitespace-nowrap text-gray-800 sm:px-6 dark:text-white/90">
                        {provider.name}
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap">
                        <Badge color="primary" size="sm">
                          {provider.provider}
                        </Badge>
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap text-gray-700 dark:text-gray-400">
                        {provider.model}
                      </TableCell>
                      <TableCell className="max-w-xs px-4 py-4 text-start text-sm break-all text-gray-700 dark:text-gray-400">
                        {provider.base_url}
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap text-gray-700 dark:text-gray-400">
                        {formatDate(provider.created_at)}
                      </TableCell>
                      <TableCell className="px-4 py-4 text-end whitespace-nowrap sm:px-6">
                        <div className="flex justify-end gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() =>
                              void navigate(`/llm-providers/${provider.id}/edit`)
                            }
                          >
                            <PencilIcon />
                            {tc("edit")}
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void handleDelete(provider)}
                            disabled={deletingId === provider.id}
                          >
                            <TrashBinIcon />
                            {deletingId === provider.id
                              ? t("deleting")
                              : tc("delete")}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>
    </>
  );
}
