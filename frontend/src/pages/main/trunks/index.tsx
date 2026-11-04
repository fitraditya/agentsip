import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  AlertIcon,
  CallIcon,
  PencilIcon,
  PlusIcon,
  TrashBinIcon,
} from "@/icons";
import {
  ApiError,
  llmProvidersApi,
  sttProvidersApi,
  trunksApi,
  ttsProvidersApi,
  type LlmProvider,
  type SttProvider,
  type Trunk,
  type TtsProvider,
} from "@/lib/api";
import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router";

const TRUNK_LIMIT = 3;

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

function providerLabel(provider: string): string {
  switch (provider) {
    case "elevenlabs":
      return "ElevenLabs";
    case "openai_compatible":
      return "OpenAI-compatible";
    case "pgvector":
      return "pgvector";
    default:
      return provider;
  }
}

export default function TrunksListPage() {
  const [trunks, setTrunks] = useState<Trunk[]>([]);
  const [sttProviders, setSttProviders] = useState<SttProvider[]>([]);
  const [llmProviders, setLlmProviders] = useState<LlmProvider[]>([]);
  const [ttsProviders, setTtsProviders] = useState<TtsProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [trunksData, sttData, llmData, ttsData] = await Promise.all([
        trunksApi.list(),
        sttProvidersApi.list(),
        llmProvidersApi.list(),
        ttsProvidersApi.list(),
      ]);
      setTrunks(trunksData);
      setSttProviders(sttData);
      setLlmProviders(llmData);
      setTtsProviders(ttsData);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "Couldn't load trunks. Please try again.",
      );
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  // Cross-reference provider_id -> name client-side, same pattern as the
  // Calls page's trunk_id -> trunk.name lookup — the trunk's resolved
  // stt/llm/tts info carries the provider kind, not its user-facing name.
  const sttName = (providerId: string) =>
    sttProviders.find((p) => p.id === providerId)?.name;
  const llmName = (providerId: string) =>
    llmProviders.find((p) => p.id === providerId)?.name;
  const ttsName = (providerId: string) =>
    ttsProviders.find((p) => p.id === providerId)?.name;

  const handleDelete = async (trunk: Trunk) => {
    const confirmed = window.confirm(
      `Delete trunk "${trunk.name}"? This cannot be undone.`,
    );
    if (!confirmed) return;

    setDeletingId(trunk.id);
    setError(null);
    try {
      await trunksApi.delete(trunk.id);
      await loadData();
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "Couldn't delete the trunk. Please try again.",
      );
    } finally {
      setDeletingId(null);
    }
  };

  const atLimit = trunks.length >= TRUNK_LIMIT;

  return (
    <div>
      <PageMeta
        title="Trunks | agentsip"
        description="Manage SIP trunks for the agentsip backend."
      />
      <PageBreadcrumb pageTitle="Trunks" />

      <div className="rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/3">
        <div className="flex flex-wrap items-center justify-between gap-3 px-6 py-5">
          <div>
            <h3 className="text-base font-medium text-gray-800 dark:text-white/90">
              Trunks
            </h3>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              Up to {TRUNK_LIMIT} trunks per deployment. Each trunk
              configures SIP auth and the STT/LLM/TTS providers used to
              answer its calls.
            </p>
          </div>
          {atLimit ? (
            <Button disabled>
              <PlusIcon />
              Create trunk
            </Button>
          ) : (
            <Button render={<Link to="/trunks/new" />} nativeButton={false}>
              <PlusIcon />
              Create trunk
            </Button>
          )}
        </div>

        <div className="border-t border-gray-100 dark:border-gray-800">
          {error && (
            <div className="px-6 pt-5">
              <Alert variant="destructive">
                <AlertIcon />
                <AlertTitle>Something went wrong</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            </div>
          )}

          {loading ? (
            <div className="flex items-center justify-center gap-2 px-6 py-16 text-sm text-gray-500 dark:text-gray-400">
              <Spinner /> Loading trunks...
            </div>
          ) : trunks.length === 0 ? (
            <div className="flex flex-col items-center justify-center px-6 py-16 text-center">
              <CallIcon className="size-8 text-gray-300 dark:text-gray-600" />
              <p className="mt-3 text-sm font-medium text-gray-800 dark:text-white/90">
                No trunks yet
              </p>
              <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                Create a trunk to start routing calls to an agent.
              </p>
            </div>
          ) : (
            <div className="max-w-full overflow-x-auto">
              <Table>
                <TableHeader className="border-b border-gray-100 dark:border-white/5">
                  <TableRow>
                    <TableCell
                      isHeader
                      className="px-6 py-3 text-start text-xs font-medium text-gray-500 dark:text-gray-400"
                    >
                      Name
                    </TableCell>
                    <TableCell
                      isHeader
                      className="px-6 py-3 text-start text-xs font-medium text-gray-500 dark:text-gray-400"
                    >
                      Providers
                    </TableCell>
                    <TableCell
                      isHeader
                      className="px-6 py-3 text-start text-xs font-medium text-gray-500 dark:text-gray-400"
                    >
                      Answer mode
                    </TableCell>
                    <TableCell
                      isHeader
                      className="px-6 py-3 text-start text-xs font-medium text-gray-500 dark:text-gray-400"
                    >
                      Max calls
                    </TableCell>
                    <TableCell
                      isHeader
                      className="px-6 py-3 text-start text-xs font-medium text-gray-500 dark:text-gray-400"
                    >
                      Created
                    </TableCell>
                    <TableCell
                      isHeader
                      className="px-6 py-3 text-end text-xs font-medium text-gray-500 dark:text-gray-400"
                    >
                      Actions
                    </TableCell>
                  </TableRow>
                </TableHeader>
                <TableBody className="divide-y divide-gray-100 dark:divide-white/5">
                  {trunks.map((trunk) => (
                    <TableRow key={trunk.id}>
                      <TableCell className="px-6 py-4 text-sm font-medium text-gray-800 dark:text-white/90">
                        {trunk.name}
                      </TableCell>
                      <TableCell className="px-6 py-4">
                        <div className="flex flex-wrap gap-1.5">
                          <Badge color="info" size="sm">
                            STT:{" "}
                            {sttName(trunk.stt_provider_id) ??
                              providerLabel(trunk.stt.provider)}
                          </Badge>
                          <Badge color="primary" size="sm">
                            LLM:{" "}
                            {llmName(trunk.llm_provider_id) ??
                              providerLabel(trunk.llm.provider)}
                          </Badge>
                          <Badge color="success" size="sm">
                            TTS:{" "}
                            {ttsName(trunk.tts_provider_id) ??
                              providerLabel(trunk.tts.provider)}
                          </Badge>
                        </div>
                      </TableCell>
                      <TableCell className="px-6 py-4">
                        <Badge color="gray" className="capitalize">
                          {trunk.answer_mode}
                        </Badge>
                      </TableCell>
                      <TableCell className="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
                        {trunk.max_concurrent_calls}
                      </TableCell>
                      <TableCell className="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
                        {formatDate(trunk.created_at)}
                      </TableCell>
                      <TableCell className="px-6 py-4 text-end">
                        <div className="flex justify-end gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            render={<Link to={`/trunks/${trunk.id}/edit`} />}
                            nativeButton={false}
                          >
                            <PencilIcon /> Edit
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void handleDelete(trunk)}
                            disabled={deletingId === trunk.id}
                          >
                            <TrashBinIcon />
                            {deletingId === trunk.id
                              ? "Deleting..."
                              : "Delete"}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
