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
  type Trunk,
  type TtsProvider,
} from "@/lib/api";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { trunkFormToRequest, trunkToFormDefaults, type TrunkFormData } from "../schema";
import TrunkForm from "../trunk-form";

function friendlyError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "TRUNK_LIMIT_EXCEEDED") {
      return "This deployment already has the maximum number of trunks (3). Delete one before creating another.";
    }
    return error.message;
  }
  return "Something went wrong. Please try again.";
}

export default function TrunkEditPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [trunk, setTrunk] = useState<Trunk | null>(null);
  const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([]);
  const [sttProviders, setSttProviders] = useState<SttProvider[]>([]);
  const [llmProviders, setLlmProviders] = useState<LlmProvider[]>([]);
  const [ttsProviders, setTtsProviders] = useState<TtsProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [submitting, setSubmitting] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;
    const load = async () => {
      setLoading(true);
      setLoadError(null);
      try {
        const [trunkData, kbData, sttData, llmData, ttsData] =
          await Promise.all([
            trunksApi.get(id),
            knowledgeBasesApi.list(),
            sttProvidersApi.list(),
            llmProvidersApi.list(),
            ttsProvidersApi.list(),
          ]);
        setTrunk(trunkData);
        setKnowledgeBases(kbData);
        setSttProviders(sttData);
        setLlmProviders(llmData);
        setTtsProviders(ttsData);
      } catch (err) {
        setLoadError(
          err instanceof ApiError ? err.message : "Couldn't load this trunk.",
        );
      } finally {
        setLoading(false);
      }
    };
    void load();
  }, [id]);

  const handleSubmit = async (data: TrunkFormData) => {
    if (!id) return;
    setSubmitting(true);
    setServerError(null);
    try {
      const req = trunkFormToRequest(data);
      await trunksApi.update(id, req);
      void navigate("/trunks");
    } catch (error) {
      setServerError(friendlyError(error));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div>
      <PageMeta
        title="Edit Trunk | agentsip"
        description="Edit an existing SIP trunk."
      />

      <Link
        to="/trunks"
        className="mb-4 inline-flex items-center gap-1 text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
      >
        <ChevronLeftIcon className="rtl:rotate-180" />
        Back to Trunks
      </Link>

      <PageBreadcrumb pageTitle={trunk ? `Edit ${trunk.name}` : "Edit Trunk"} />

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
      ) : trunk ? (
        <TrunkForm
          key={trunk.id}
          mode="edit"
          defaultValues={trunkToFormDefaults(trunk)}
          configured={{
            digest_configured: trunk.sip_auth.digest_configured,
          }}
          knowledgeBases={knowledgeBases}
          sttProviders={sttProviders}
          llmProviders={llmProviders}
          ttsProviders={ttsProviders}
          serverError={serverError}
          submitting={submitting}
          onSubmit={(data) => void handleSubmit(data)}
          onCancel={() => void navigate("/trunks")}
        />
      ) : !loadError ? (
        <Card>
          <CardContent>
            <p className="text-sm text-gray-500 dark:text-gray-400">
              Trunk not found.
            </p>
            <Button
              className="mt-4"
              variant="outline"
              render={<Link to="/trunks" />}
              nativeButton={false}
            >
              Back to Trunks
            </Button>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
