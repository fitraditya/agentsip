import { Spinner } from "@/components/ui/spinner";
import { useAuth } from "@/contexts";
import { Navigate, Outlet, useLocation } from "react-router";

/**
 * Wraps dashboard routes in App.tsx. Unauthenticated users are redirected to
 * /signin; the attempted location is kept in router state so sign-in could
 * optionally send them back (not currently used, but harmless to carry).
 */
export default function ProtectedRoute() {
  const { isAuthenticated, isInitializing } = useAuth();
  const location = useLocation();

  if (isInitializing) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner className="size-6 text-gray-400" />
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/signin" replace state={{ from: location }} />;
  }

  return <Outlet />;
}
