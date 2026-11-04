import { PageMeta } from "@/components/shared/page-meta";
import SignUp from "./index";

export default function SignUpPage() {
  return (
    <>
      <PageMeta
        title="Sign Up | agentsip"
        description="Create an agentsip admin account."
      />
      <SignUp />
    </>
  );
}
