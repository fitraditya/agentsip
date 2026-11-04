import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import { AlertIcon, PlusIcon } from "@/icons";
import { ApiError, knowledgeBasesApi, type KnowledgeBase } from "@/lib/api";
import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";

export default function KnowledgeBasesListPage() {
  const t = useTranslations("knowledgeBases");
  const tc = useTranslations("common");
  const navigate = useNavigate();

  const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadKnowledgeBases = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await knowledgeBasesApi.list();
      setKnowledgeBases(data);
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : t("errors.loadFailed"),
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadKnowledgeBases();
  }, []);

  return (
    <>
      <PageMeta
        title="Knowledge Bases | AgentSIP"
        description="Manage the knowledge bases available to your trunks."
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
          <Button
            size="sm"
            render={<Link to="/knowledge-bases/new" />}
            nativeButton={false}
          >
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
                    {t("table.embeddingProvider")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-start text-sm font-normal text-gray-500 dark:text-gray-400"
                  >
                    {t("table.vectorStore")}
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
                      colSpan={5}
                      className="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400"
                    >
                      {tc("loading")}
                    </TableCell>
                  </TableRow>
                ) : knowledgeBases.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      className="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400"
                    >
                      {t("empty")}
                    </TableCell>
                  </TableRow>
                ) : (
                  knowledgeBases.map((kb) => (
                    <TableRow
                      key={kb.id}
                      className="cursor-pointer hover:bg-gray-50 dark:hover:bg-white/5"
                      onClick={() => void navigate(`/knowledge-bases/${kb.id}`)}
                    >
                      <TableCell className="px-5 py-4 text-start text-sm font-medium whitespace-nowrap text-gray-800 sm:px-6 dark:text-white/90">
                        {kb.name}
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap text-gray-700 dark:text-gray-400">
                        {kb.embedding.provider}
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap text-gray-700 dark:text-gray-400">
                        {kb.vector_store.custom_dsn_set
                          ? t("vectorStore.custom")
                          : t("vectorStore.platformManaged")}
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap text-gray-700 dark:text-gray-400">
                        {new Date(kb.created_at).toLocaleDateString()}
                      </TableCell>
                      <TableCell className="px-4 py-4 text-end whitespace-nowrap sm:px-6">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={(event) => {
                            event.stopPropagation();
                            void navigate(`/knowledge-bases/${kb.id}`);
                          }}
                        >
                          {t("table.view")}
                        </Button>
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
