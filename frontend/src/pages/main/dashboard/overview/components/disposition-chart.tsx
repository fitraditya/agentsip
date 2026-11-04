import { useTheme } from "@/contexts";
import { useTranslations } from "@/i18n";
import type { Call, CallDisposition } from "@/lib/api";
import type { ApexOptions } from "apexcharts";
import ReactApexChart from "react-apexcharts";

interface DispositionChartProps {
  calls: Call[];
  loading?: boolean;
}

const DISPOSITIONS: CallDisposition[] = [
  "ANSWERED",
  "NO_ANSWER",
  "BUSY",
  "FAILED",
  "CANCELLED",
  "IN_PROGRESS",
];

export default function DispositionChart({
  calls,
  loading,
}: DispositionChartProps) {
  const { theme } = useTheme();
  const t = useTranslations("dashboard.dispositionChart");
  const td = useTranslations("calls.disposition");

  const counts = DISPOSITIONS.map(
    (disposition) =>
      calls.filter((call) => call.disposition === disposition).length,
  );

  const options: ApexOptions = {
    colors: ["#465FFF"],
    chart: {
      fontFamily: "Outfit, sans-serif",
      type: "bar",
      toolbar: { show: false },
    },
    plotOptions: {
      bar: {
        horizontal: false,
        columnWidth: "45%",
        borderRadius: 6,
        borderRadiusApplication: "around",
      },
    },
    dataLabels: { enabled: false },
    stroke: { show: false },
    xaxis: {
      categories: DISPOSITIONS.map((disposition) => td(disposition)),
      axisBorder: { show: false },
      axisTicks: { show: false },
      labels: {
        style: {
          colors: theme === "dark" ? "#94A3B8" : "#364153",
          fontFamily: "Outfit, sans-serif",
        },
      },
    },
    yaxis: {
      forceNiceScale: true,
      labels: { show: true },
    },
    grid: {
      borderColor: theme === "dark" ? "#1E2640" : "#F3F4F6",
      yaxis: { lines: { show: true } },
      xaxis: { lines: { show: false } },
    },
    legend: { show: false },
    tooltip: {
      y: {
        formatter: (val: number) => `${val}`,
      },
    },
  };

  const series = [{ name: t("series"), data: counts }];

  if (loading) {
    return (
      <div className="h-[310px] animate-pulse rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/3" />
    );
  }

  return (
    <div className="overflow-hidden rounded-2xl border border-gray-200 bg-white px-5 pt-5 pb-5 sm:px-6 sm:pt-6 sm:pb-5 dark:border-gray-800 dark:bg-white/3">
      <div className="mb-4">
        <h3 className="text-base font-medium text-gray-800 dark:text-white/90">
          {t("title")}
        </h3>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {t("subtitle")}
        </p>
      </div>
      <ReactApexChart
        options={options}
        series={series}
        type="bar"
        height={250}
      />
    </div>
  );
}
