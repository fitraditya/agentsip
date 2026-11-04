import { PageMeta } from "@/components/shared/page-meta";
import SignIn from "./index";

export default function SignInPage() {
  return (
    <>
      <PageMeta
        title="Sign In | agentsip"
        description="Sign in to your agentsip admin dashboard."
      />
      <SignIn />
    </>
  );
}
