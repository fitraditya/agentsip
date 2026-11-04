import { Spinner } from "@/components/ui/spinner";
import { useAuth } from "@/contexts";
import { Navigate, Outlet } from "react-router";

/**
 * Wraps /signin and /signup. An already-authenticated user hitting either is
 * sent to the dashboard home instead of seeing the auth forms again.
 */
export default function GuestRoute() {
  const { isAuthenticated, isInitializing } = useAuth();

  if (isInitializing) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner className="size-6 text-gray-400" />
      </div>
    );
  }

  if (isAuthenticated) {
    return <Navigate to="/" replace />;
  }

  return <Outlet />;
}
