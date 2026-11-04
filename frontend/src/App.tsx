import GuestRoute from "@/components/auth/guest-route";
import ProtectedRoute from "@/components/auth/protected-route";
import AppLayout from "@/components/layout/app-layout";
import CallDetailPage from "@/pages/main/calls/detail";
import CallsListPage from "@/pages/main/calls/list";
import DashboardOverviewPage from "@/pages/main/dashboard/overview";
import KnowledgeBaseDetailPage from "@/pages/main/knowledge-bases/detail";
import KnowledgeBasesListPage from "@/pages/main/knowledge-bases/list";
import KnowledgeBaseNewPage from "@/pages/main/knowledge-bases/new";
import LlmProviderEditPage from "@/pages/main/llm-providers/edit";
import LlmProvidersListPage from "@/pages/main/llm-providers/list";
import LlmProviderNewPage from "@/pages/main/llm-providers/new";
import { Error404Page } from "@/pages/main/pages";
import ProfilePage from "@/pages/main/profile";
import SttProviderEditPage from "@/pages/main/stt-providers/edit";
import SttProvidersListPage from "@/pages/main/stt-providers/list";
import SttProviderNewPage from "@/pages/main/stt-providers/new";
import TrunksListPage from "@/pages/main/trunks";
import TrunkEditPage from "@/pages/main/trunks/edit";
import TrunkNewPage from "@/pages/main/trunks/new";
import TtsProviderEditPage from "@/pages/main/tts-providers/edit";
import TtsProvidersListPage from "@/pages/main/tts-providers/list";
import TtsProviderNewPage from "@/pages/main/tts-providers/new";
import { AuthLayout, SignInPage, SignUpPage } from "@/pages/others/auth";
import "flatpickr/dist/flatpickr.css";
import { Route, Routes } from "react-router";

export default function App() {
  return (
    <Routes>
      {/* Dashboard Layout — auth required */}
      <Route element={<ProtectedRoute />}>
        <Route element={<AppLayout />}>
          <Route index element={<DashboardOverviewPage />} />

          <Route path="/trunks" element={<TrunksListPage />} />
          <Route path="/trunks/new" element={<TrunkNewPage />} />
          <Route path="/trunks/:id/edit" element={<TrunkEditPage />} />

          <Route path="/knowledge-bases" element={<KnowledgeBasesListPage />} />
          <Route
            path="/knowledge-bases/new"
            element={<KnowledgeBaseNewPage />}
          />
          <Route
            path="/knowledge-bases/:id"
            element={<KnowledgeBaseDetailPage />}
          />

          <Route path="/stt-providers" element={<SttProvidersListPage />} />
          <Route path="/stt-providers/new" element={<SttProviderNewPage />} />
          <Route
            path="/stt-providers/:id/edit"
            element={<SttProviderEditPage />}
          />

          <Route path="/llm-providers" element={<LlmProvidersListPage />} />
          <Route path="/llm-providers/new" element={<LlmProviderNewPage />} />
          <Route
            path="/llm-providers/:id/edit"
            element={<LlmProviderEditPage />}
          />

          <Route path="/tts-providers" element={<TtsProvidersListPage />} />
          <Route path="/tts-providers/new" element={<TtsProviderNewPage />} />
          <Route
            path="/tts-providers/:id/edit"
            element={<TtsProviderEditPage />}
          />

          <Route path="/calls" element={<CallsListPage />} />
          <Route path="/calls/:id" element={<CallDetailPage />} />

          {/* Linked from the header's user dropdown (Edit profile). Lets the
              signed-in user change their own email and password. */}
          <Route path="/profile" element={<ProfilePage />} />
        </Route>
      </Route>

      {/* Auth Layout — guest only; redirects to "/" if already signed in */}
      <Route element={<GuestRoute />}>
        <Route element={<AuthLayout />}>
          <Route path="/signin" element={<SignInPage />} />
          <Route path="/signup" element={<SignUpPage />} />
        </Route>
      </Route>

      {/* Full-width / Error / Status Pages */}
      <Route path="/error-404" element={<Error404Page />} />

      {/* Fallback Route */}
      <Route path="*" element={<Error404Page />} />
    </Routes>
  );
}
