import { useTranslations } from "@/i18n";
import { BoxIcon, CallIcon, DatabaseIcon, PluginIcon } from "@/icons";
import type { Call } from "@/lib/api";
import StatCard from "./stat-card";

export interface DashboardStats {
  trunkCount: number;
  knowledgeBaseCount: number;
  /** STT + LLM + TTS provider configs combined. */
  providerCount: number;
  callsToday: number;
  recentCalls: Call[];
}

interface StatCardsProps {
  stats: DashboardStats | null;
  loading: boolean;
}

export default function StatCards({ stats, loading }: StatCardsProps) {
  const t = useTranslations("dashboard.stats");

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 md:gap-6 xl:grid-cols-4">
      <StatCard
        icon={<BoxIcon />}
        title={t("activeTrunks")}
        value={stats?.trunkCount ?? 0}
        loading={loading}
      />
      <StatCard
        icon={<DatabaseIcon />}
        title={t("knowledgeBases")}
        value={stats?.knowledgeBaseCount ?? 0}
        loading={loading}
      />
      <StatCard
        icon={<CallIcon />}
        title={t("callsToday")}
        value={stats?.callsToday ?? 0}
        caption={t("callsTodayCaption")}
        loading={loading}
      />
      <StatCard
        icon={<PluginIcon />}
        title={t("providers")}
        value={stats?.providerCount ?? 0}
        loading={loading}
      />
    </div>
  );
}
