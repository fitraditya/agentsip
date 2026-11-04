import { SidebarHeader } from "@/components/ui/sidebar";
import { Link } from "react-router";

interface AppSidebarHeaderProps {
  showContent: boolean;
}

export default function AppSidebarHeader({
  showContent,
}: AppSidebarHeaderProps) {
  return (
    <SidebarHeader className="px-5 py-8">
      <Link to="/" className="flex items-center gap-2.5">
        <img src="/images/logo/logo-icon.svg" alt="agentsip" className="size-8 shrink-0" />
        {showContent && (
          <span className="text-title-sm font-semibold text-gray-900 dark:text-white">
            AgentSIP
          </span>
        )}
      </Link>
    </SidebarHeader>
  );
}
