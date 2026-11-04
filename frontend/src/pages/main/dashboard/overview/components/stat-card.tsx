import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/utils";
import type { ReactNode } from "react";

interface StatCardProps {
  icon: ReactNode;
  title: string;
  value: string | number;
  caption?: string;
  loading?: boolean;
  className?: string;
}

export default function StatCard({
  icon,
  title,
  value,
  caption,
  loading,
  className,
}: StatCardProps) {
  return (
    <div
      className={cn(
        "rounded-2xl border border-gray-200 bg-white p-5 md:p-6 dark:border-gray-800 dark:bg-white/3",
        className,
      )}
    >
      <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-white/90 [&>svg]:size-6">
        {icon}
      </div>

      <div className="mt-5">
        <span className="text-sm text-gray-500 dark:text-gray-400">
          {title}
        </span>
        {loading ? (
          <Skeleton className="mt-2 h-8 w-16" />
        ) : (
          <h4 className="mt-2 text-title-sm font-bold text-gray-800 dark:text-white/90">
            {value}
          </h4>
        )}
        {caption && (
          <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
            {caption}
          </p>
        )}
      </div>
    </div>
  );
}
