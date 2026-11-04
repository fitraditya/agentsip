import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useTranslations } from "@/i18n";
import { AlertIcon, ChevronLeftIcon } from "@/icons";
import {
  ApiError,
  callsApi,
  type Call,
  type CallDirection,
  type CallDisposition,
} from "@/lib/api";
import { cn } from "@/utils";
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";

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

function DetailField({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div>
      <p className="text-xs text-gray-500 dark:text-gray-400">{label}</p>
      <p
        className={cn(
          "mt-1 text-sm font-medium text-gray-800 dark:text-white/90",
          mono && "font-mono break-all",
        )}
      >
        {value}
      </p>
    </div>
  );
}

export default function CallDetailPage() {
  const { id } = useParams<{ id: string }>();
  const t = useTranslations("calls.detail");
  const td = useTranslations("calls.direction");
  const tdisp = useTranslations("calls.disposition");
  const tc = useTranslations("common");

  const [call, setCall] = useState<Call | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;

    async function load() {
      setLoading(true);
      setError(null);
      try {
        const data = await callsApi.get(id as string);
        if (!cancelled) setCall(data);
      } catch (err) {
        if (!cancelled) {
          setError(
            err instanceof ApiError
              ? err.message
              : "Something went wrong. Please try again.",
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void load();

    return () => {
      cancelled = true;
    };
  }, [id]);

  return (
    <>
      <PageMeta title="Call Detail | AgentSIP" description="Call details." />

      <Link
        to="/calls"
        className="mb-4 inline-flex items-center gap-1 text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
      >
        <ChevronLeftIcon className="rtl:rotate-180" />
        {t("backLink")}
      </Link>

      <PageBreadcrumb pageTitle={t("overviewTitle")} />

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
      ) : call ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("overviewTitle")}</CardTitle>
          </CardHeader>
          <CardContent className="grid grid-cols-1 gap-5 sm:grid-cols-2">
            <DetailField label={t("sipCallId")} value={call.sip_call_id} mono />
            <DetailField label={t("trunkId")} value={call.trunk_id} mono />
            <DetailField label={t("callerNumber")} value={call.caller_number} />
            <DetailField label={t("calleeNumber")} value={call.callee_number} />

            <div>
              <p className="text-xs text-gray-500 dark:text-gray-400">
                {t("direction")}
              </p>
              <Badge
                className="mt-1"
                color={DIRECTION_BADGE_COLOR[call.direction]}
              >
                {td(call.direction)}
              </Badge>
            </div>
            <div>
              <p className="text-xs text-gray-500 dark:text-gray-400">
                {t("disposition")}
              </p>
              <Badge
                className="mt-1"
                color={DISPOSITION_BADGE_COLOR[call.disposition]}
              >
                {tdisp(call.disposition)}
              </Badge>
            </div>

            <DetailField
              label={t("startTime")}
              value={new Date(call.start_time).toLocaleString()}
            />
            <DetailField
              label={t("answerTime")}
              value={
                call.answer_time
                  ? new Date(call.answer_time).toLocaleString()
                  : "—"
              }
            />
            <DetailField
              label={t("endTime")}
              value={
                call.end_time ? new Date(call.end_time).toLocaleString() : "—"
              }
            />
            <DetailField
              label={t("duration")}
              value={formatDuration(call.duration_seconds)}
            />
            <DetailField
              label={t("createdAt")}
              value={new Date(call.created_at).toLocaleString()}
            />
            <DetailField
              label={t("updatedAt")}
              value={new Date(call.updated_at).toLocaleString()}
            />

            <div className="sm:col-span-2">
              <p className="text-xs text-gray-500 dark:text-gray-400">
                {t("recording")}
              </p>
              {call.recording_url ? (
                <audio
                  controls
                  src={call.recording_url}
                  className="mt-2 w-full"
                />
              ) : (
                <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {t("noRecording")}
                </p>
              )}
            </div>
          </CardContent>
        </Card>
      ) : null}
    </>
  );
}
