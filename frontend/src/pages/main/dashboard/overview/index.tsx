import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { PageMeta } from "@/components/shared/page-meta";
import { useTranslations } from "@/i18n";
import { AlertIcon } from "@/icons";
import {
  ApiError,
  callsApi,
  knowledgeBasesApi,
  llmProvidersApi,
  sttProvidersApi,
  trunksApi,
  ttsProvidersApi,
} from "@/lib/api";
import { useEffect, useState } from "react";
import DispositionChart from "./components/disposition-chart";
import StatCards, { type DashboardStats } from "./components/stat-cards";

function isSameLocalDay(isoString: string, reference: Date): boolean {
  const date = new Date(isoString);
  return (
    date.getFullYear() === reference.getFullYear() &&
    date.getMonth() === reference.getMonth() &&
    date.getDate() === reference.getDate()
  );
}

export default function DashboardOverviewPage() {
  const t = useTranslations("dashboard");
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setLoading(true);
      setError(null);
      try {
        const [trunks, knowledgeBases, sttProviders, llmProviders, ttsProviders, recentCalls] =
          await Promise.all([
            trunksApi.list(),
            knowledgeBasesApi.list(),
            sttProvidersApi.list(),
            llmProvidersApi.list(),
            ttsProvidersApi.list(),
            callsApi.list({ limit: 100 }),
          ]);

        if (cancelled) return;

        const today = new Date();
        const callsToday = recentCalls.filter((call) =>
          isSameLocalDay(call.start_time, today),
        ).length;

        setStats({
          trunkCount: trunks.length,
          knowledgeBaseCount: knowledgeBases.length,
          providerCount:
            sttProviders.length + llmProviders.length + ttsProviders.length,
          callsToday,
          recentCalls,
        });
      } catch (err) {
        if (cancelled) return;
        setError(
          err instanceof ApiError
            ? err.message
            : "Something went wrong. Please try again.",
        );
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void load();

    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <>
      <PageMeta
        title="Dashboard | AgentSIP"
        description="An overview of your AgentSIP deployment."
      />

      <div className="mb-6">
        <h2 className="text-title-lg font-semibold text-gray-800 dark:text-white/90">
          {t("title")}
        </h2>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {t("subtitle")}
        </p>
      </div>

      {error && (
        <Alert variant="destructive" className="mb-6">
          <AlertIcon />
          <AlertTitle>{t("errors.loadFailed")}</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="grid grid-cols-12 gap-4 md:gap-6">
        <div className="col-span-12">
          <StatCards stats={stats} loading={loading} />
        </div>

        <div className="col-span-12">
          <DispositionChart
            calls={stats?.recentCalls ?? []}
            loading={loading}
          />
        </div>
      </div>
    </>
  );
}
