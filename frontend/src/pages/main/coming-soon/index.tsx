import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";

interface ComingSoonPageProps {
  title: string;
}

/**
 * Temporary placeholder for dashboard routes whose real feature page hasn't
 * landed yet (Dashboard / API Keys / Trunks / Knowledge Bases / Calls).
 *
 * Other agents: replace the <Route> in src/App.tsx pointing here with your
 * real page component once it exists. This file can be deleted once nothing
 * references it anymore.
 */
export default function ComingSoonPage({ title }: ComingSoonPageProps) {
  return (
    <div>
      <PageMeta
        title={`${title} | agentsip`}
        description={`The ${title} page is coming soon.`}
      />
      <PageBreadcrumb pageTitle={title} />
      <div className="flex flex-col items-center justify-center rounded-2xl border border-gray-200 bg-white px-6 py-16 text-center dark:border-gray-800 dark:bg-white/[0.03]">
        <p className="text-lg font-medium text-gray-800 dark:text-white/90">
          {title}
        </p>
        <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">
          Coming soon.
        </p>
      </div>
    </div>
  );
}
