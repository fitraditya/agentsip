import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { AlertIcon, ChevronLeftIcon } from "@/icons";
import {
  ApiError,
  knowledgeBasesApi,
  llmProvidersApi,
  sttProvidersApi,
  trunksApi,
  ttsProvidersApi,
  type KnowledgeBase,
  type LlmProvider,
  type SttProvider,
  type TtsProvider,
} from "@/lib/api";
import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import {
  emptyTrunkFormDefaults,
  trunkFormToRequest,
  type TrunkFormData,
} from "../schema";
import TrunkForm from "../trunk-form";

const TRUNK_LIMIT = 3;

function friendlyError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "TRUNK_LIMIT_EXCEEDED") {
      return "This deployment already has the maximum number of trunks (3). Delete one before creating another.";
    }
    return error.message;
  }
  return "Something went wrong. Please try again.";
}

export default function TrunkNewPage() {
  const navigate = useNavigate();

  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [trunkCount, setTrunkCount] = useState(0);
  const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([]);
  const [sttProviders, setSttProviders] = useState<SttProvider[]>([]);
  const [llmProviders, setLlmProviders] = useState<LlmProvider[]>([]);
  const [ttsProviders, setTtsProviders] = useState<TtsProvider[]>([]);

  const [submitting, setSubmitting] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      setLoadError(null);
      try {
        const [trunks, kbData, sttData, llmData, ttsData] = await Promise.all(
          [
            trunksApi.list(),
            knowledgeBasesApi.list(),
            sttProvidersApi.list(),
            llmProvidersApi.list(),
            ttsProvidersApi.list(),
          ],
        );
        setTrunkCount(trunks.length);
        setKnowledgeBases(kbData);
        setSttProviders(sttData);
        setLlmProviders(llmData);
        setTtsProviders(ttsData);
      } catch (err) {
        setLoadError(
          err instanceof ApiError
            ? err.message
            : "Couldn't load the data needed to create a trunk.",
        );
      } finally {
        setLoading(false);
      }
    };
    void load();
  }, []);

  const handleSubmit = async (data: TrunkFormData) => {
    setSubmitting(true);
    setServerError(null);
    try {
      const req = trunkFormToRequest(data);
      await trunksApi.create(req);
      void navigate("/trunks");
    } catch (error) {
      setServerError(friendlyError(error));
    } finally {
      setSubmitting(false);
    }
  };

  const atLimit = trunkCount >= TRUNK_LIMIT;

  return (
    <div>
      <PageMeta
        title="New Trunk | agentsip"
        description="Create a new SIP trunk."
      />

      <Link
        to="/trunks"
        className="mb-4 inline-flex items-center gap-1 text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
      >
        <ChevronLeftIcon className="rtl:rotate-180" />
        Back to Trunks
      </Link>

      <PageBreadcrumb pageTitle="New Trunk" />

      {loadError && (
        <Alert variant="destructive" className="mb-6">
          <AlertIcon />
          <AlertTitle>Something went wrong</AlertTitle>
          <AlertDescription>{loadError}</AlertDescription>
        </Alert>
      )}

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-16 text-sm text-gray-500 dark:text-gray-400">
          <Spinner /> Loading...
        </div>
      ) : atLimit ? (
        <Card>
          <CardContent>
            <Alert variant="warning">
              <AlertIcon />
              <AlertTitle>Trunk limit reached</AlertTitle>
              <AlertDescription>
                This deployment already has the maximum number of trunks (
                {TRUNK_LIMIT}). Delete one before creating another.
              </AlertDescription>
            </Alert>
            <Button
              className="mt-5"
              variant="outline"
              render={<Link to="/trunks" />}
              nativeButton={false}
            >
              Back to Trunks
            </Button>
          </CardContent>
        </Card>
      ) : (
        <TrunkForm
          mode="create"
          defaultValues={emptyTrunkFormDefaults()}
          knowledgeBases={knowledgeBases}
          sttProviders={sttProviders}
          llmProviders={llmProviders}
          ttsProviders={ttsProviders}
          serverError={serverError}
          submitting={submitting}
          onSubmit={(data) => void handleSubmit(data)}
          onCancel={() => void navigate("/trunks")}
        />
      )}
    </div>
  );
}
