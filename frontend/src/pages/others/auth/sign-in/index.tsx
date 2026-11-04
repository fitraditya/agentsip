import { Link, useLocation } from "react-router";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ChevronLeftIcon, CheckCircleIcon } from "@/icons";
import { cn } from "@/utils";
import AuthDivider from "../divider";
import SignInFormFields from "./form";
import SignInHeader from "./header";
import { SocialButtons } from "./social-button";

export default function SignIn() {
  const location = useLocation();
  const justRegistered = Boolean(
    (location.state as { registered?: boolean } | null)?.registered,
  );

  return (
    <div className={cn("flex w-full flex-1 flex-col lg:w-1/2")}>
      <div className="mx-auto mb-5 w-full max-w-md sm:pt-10">
        <Link
          to="/"
          className="inline-flex items-center text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
        >
          <ChevronLeftIcon className="rtl:rotate-180" />
          Back to dashboard
        </Link>
      </div>

      <div className="mx-auto flex w-full max-w-md flex-1 flex-col justify-center">
        <div>
          <SignInHeader />

          {justRegistered && (
            <Alert variant="success" className="mb-5">
              <CheckCircleIcon />
              <AlertTitle>Account created</AlertTitle>
              <AlertDescription>
                Please sign in with your new credentials.
              </AlertDescription>
            </Alert>
          )}

          <div>
            <SocialButtons />
            <AuthDivider />
            <SignInFormFields />

            <div className="mt-5">
              <p className="text-center text-sm font-normal text-gray-700 sm:text-start dark:text-gray-400">
                Don&apos;t have an account?{" "}
                <Link
                  to="/signup"
                  className="text-brand-500 hover:text-brand-600 dark:text-brand-400"
                >
                  Sign Up
                </Link>
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
