import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardFooter,
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
import { AlertIcon } from "@/icons";
import {
  ApiError,
  callsApi,
  trunksApi,
  type Call,
  type CallDirection,
  type CallDisposition,
  type Trunk,
} from "@/lib/api";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router";

const PAGE_SIZE = 25;

const DIRECTION_BADGE_COLOR: Record<CallDirection, "info" | "primary"> = {
  INBOUND: "info",
  OUTBOUND: "primary",
};

const DISPOSITION_BADGE_COLOR: Record<
  CallDisposition,
  "success" | "warning" | "error" | "info" | "gray"
> = {
  ANSWERED: "success",
  BUSY: "warning",
  NO_ANSWER: "warning",
  FAILED: "error",
  CANCELLED: "gray",
  IN_PROGRESS: "info",
};

function formatDuration(totalSeconds: number): string {
  const safeSeconds = Math.max(0, Math.floor(totalSeconds));
  const minutes = Math.floor(safeSeconds / 60);
  const seconds = safeSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

function shortId(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}…` : id;
}

export default function CallsListPage() {
  const t = useTranslations("calls");
  const tc = useTranslations("common");
  const td = useTranslations("calls.direction");
  const tdisp = useTranslations("calls.disposition");
  const navigate = useNavigate();

  const [calls, setCalls] = useState<Call[]>([]);
  const [trunks, setTrunks] = useState<Trunk[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const [callsData, trunksData] = await Promise.all([
        callsApi.list({ limit: PAGE_SIZE }),
        trunksApi.list().catch(() => [] as Trunk[]),
      ]);
      setCalls(callsData);
      setTrunks(trunksData);
      setHasMore(callsData.length === PAGE_SIZE);
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
  }, []);

  const loadMore = async () => {
    const last = calls[calls.length - 1];
    if (!last) return;
    setLoadingMore(true);
    try {
      // `before` is an opaque cursor — the backend's list response carries
      // no separate cursor field, so we pass the last row's id, which is
      // the only stable per-row identifier available to the frontend.
      const more = await callsApi.list({ limit: PAGE_SIZE, before: last.id });
      setCalls((prev) => [...prev, ...more]);
      setHasMore(more.length === PAGE_SIZE);
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : t("errors.loadFailed"),
      );
    } finally {
      setLoadingMore(false);
    }
  };

  const trunkName = (trunkId: string) =>
    trunks.find((trunk) => trunk.id === trunkId)?.name ?? shortId(trunkId);

  return (
    <>
      <PageMeta title="Calls | AgentSIP" description="Browse recent calls." />
      <PageBreadcrumb pageTitle={t("title")} />

      {error && (
        <Alert variant="destructive" className="mb-6">
          <AlertIcon />
          <AlertTitle>{t("errors.loadFailed")}</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card className="gap-0 overflow-hidden p-0">
        <CardHeader className="p-5 sm:p-6">
          <CardTitle className="mb-0">{t("title")}</CardTitle>
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
                    {t("table.caller")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-start text-sm font-normal text-gray-500 dark:text-gray-400"
                  >
                    {t("table.direction")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-start text-sm font-normal text-gray-500 dark:text-gray-400"
                  >
                    {t("table.disposition")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-start text-sm font-normal text-gray-500 dark:text-gray-400"
                  >
                    {t("table.startTime")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-start text-sm font-normal text-gray-500 dark:text-gray-400"
                  >
                    {t("table.duration")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-4 py-3 text-start text-sm font-normal text-gray-500 sm:px-6 dark:text-gray-400"
                  >
                    {t("table.trunk")}
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
                ) : calls.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400"
                    >
                      {t("empty")}
                    </TableCell>
                  </TableRow>
                ) : (
                  calls.map((call) => (
                    <TableRow
                      key={call.id}
                      className="cursor-pointer hover:bg-gray-50 dark:hover:bg-white/5"
                      onClick={() => void navigate(`/calls/${call.id}`)}
                    >
                      <TableCell className="px-5 py-4 text-start text-sm whitespace-nowrap text-gray-800 sm:px-6 dark:text-white/90">
                        {call.caller_number} → {call.callee_number}
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap">
                        <Badge
                          size="sm"
                          color={DIRECTION_BADGE_COLOR[call.direction]}
                        >
                          {td(call.direction)}
                        </Badge>
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap">
                        <Badge
                          size="sm"
                          color={DISPOSITION_BADGE_COLOR[call.disposition]}
                        >
                          {tdisp(call.disposition)}
                        </Badge>
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap text-gray-700 dark:text-gray-400">
                        {new Date(call.start_time).toLocaleString()}
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap text-gray-700 dark:text-gray-400">
                        {formatDuration(call.duration_seconds)}
                      </TableCell>
                      <TableCell className="px-4 py-4 text-start text-sm whitespace-nowrap text-gray-700 sm:px-6 dark:text-gray-400">
                        {trunkName(call.trunk_id)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </CardContent>

        {!loading && hasMore && calls.length > 0 && (
          <CardFooter className="justify-center p-4">
            <Button
              variant="outline"
              size="sm"
              onClick={() => void loadMore()}
              disabled={loadingMore}
            >
              {loadingMore ? tc("loading") : t("loadMore")}
            </Button>
          </CardFooter>
        )}
      </Card>
    </>
  );
}
