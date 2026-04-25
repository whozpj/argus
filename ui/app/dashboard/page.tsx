"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import {
  AlertTriangle,
  CheckCircle,
  Copy,
  Database,
  RefreshCw,
  Terminal,
  Trash2,
  Zap,
} from "lucide-react";

import Shell, { useShell } from "@/components/Shell";
import { fetchBaselines, deleteModel } from "@/lib/api";
import type { BaselineModel, BaselinesResponse } from "@/lib/types";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

type Tab = "overview" | "models" | "alerts";

function DashboardInner() {
  const { selectedProject } = useShell();
  const params = useSearchParams();
  const tab = (params.get("tab") ?? "overview") as Tab;

  const [data, setData] = useState<BaselinesResponse | null>(null);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  const projectID = selectedProject?.id ?? "";

  const loadBaselines = useCallback(
    (isRefresh = false) => {
      if (isRefresh) setRefreshing(true);
      else setLoading(true);
      setFetchError(null);

      fetchBaselines(projectID || undefined)
        .then(setData)
        .catch((e: Error) => setFetchError(e.message))
        .finally(() => {
          setLoading(false);
          setRefreshing(false);
        });
    },
    [projectID],
  );

  useEffect(() => {
    loadBaselines();
  }, [loadBaselines]);

  // Auto-refresh every 60s (same cadence as server detector).
  useEffect(() => {
    if (!projectID) return;
    const id = setInterval(() => loadBaselines(true), 60_000);
    return () => clearInterval(id);
  }, [projectID, loadBaselines]);

  const alertedModels = data?.baselines.filter((b) => b.drift_alerted) ?? [];

  return (
    <div className="space-y-6" data-testid="dashboard">
      {/* Header row */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-[22px] font-medium text-[#202124]">
            {tab === "models" ? "Models" : tab === "alerts" ? "Alerts" : "Overview"}
          </h1>
          {selectedProject && (
            <p className="text-xs text-[#5f6368] mt-0.5">
              Project: <span className="font-mono">{selectedProject.name}</span>
            </p>
          )}
        </div>
        <button
          onClick={() => loadBaselines(true)}
          disabled={refreshing}
          aria-label="Refresh"
          className="inline-flex items-center gap-2 h-8 px-3 rounded-md border border-[#dadce0] bg-white text-sm text-[#5f6368] hover:bg-[#f1f3f4] disabled:opacity-50"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          Refresh
        </button>
      </div>

      {loading ? (
        <LoadingSkeleton />
      ) : fetchError ? (
        <ErrorBanner message={fetchError} onRetry={() => loadBaselines()} />
      ) : data ? (
        data.total_events === 0 ? (
          <Onboarding onRefresh={() => loadBaselines(true)} refreshing={refreshing} />
        ) : (
          <>
            {/* Overview: cards + banner + table */}
            {tab === "overview" && (
              <>
                <SummaryCards data={data} alertedCount={alertedModels.length} />
                {alertedModels.length > 0 && <DriftBanner models={alertedModels} />}
                {alertedModels.length === 0 && data.baselines.some((b) => b.drift_score > 0) && <AllClear />}
                <ModelsCard baselines={data.baselines} projectName={selectedProject?.name} projectID={projectID} onDelete={() => loadBaselines(true)} />
              </>
            )}
            {/* Models: cards + table only */}
            {tab === "models" && (
              <>
                <SummaryCards data={data} alertedCount={alertedModels.length} />
                <ModelsCard baselines={data.baselines} projectName={selectedProject?.name} projectID={projectID} onDelete={() => loadBaselines(true)} />
              </>
            )}
            {/* Alerts: banner + table filtered to drift-alerted rows */}
            {tab === "alerts" && (
              <>
                {alertedModels.length > 0 ? (
                  <>
                    <DriftBanner models={alertedModels} />
                    <ModelsCard baselines={alertedModels} projectName={selectedProject?.name} projectID={projectID} onDelete={() => loadBaselines(true)} />
                  </>
                ) : (
                  <AllClear />
                )}
              </>
            )}
          </>
        )
      ) : null}
    </div>
  );
}

export default function DashboardPage() {
  return (
    <Suspense>
      <Shell>
        <Suspense>
          <DashboardInner />
        </Suspense>
      </Shell>
    </Suspense>
  );
}

// ─── Sub-components ────────────────────────────────────────────────────────────

function SummaryCards({ data, alertedCount }: { data: BaselinesResponse; alertedCount: number }) {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
      <StatCard label="Total Events" value={data.total_events.toLocaleString()} icon={<Database className="h-4 w-4" />} />
      <StatCard label="Models" value={data.baselines.length} icon={<Zap className="h-4 w-4" />} />
      <StatCard label="Baselines Ready" value={data.baselines.filter((b) => b.is_ready).length} icon={<CheckCircle className="h-4 w-4" />} />
      <StatCard label="Active Alerts" value={alertedCount} icon={<AlertTriangle className="h-4 w-4" />} highlight={alertedCount > 0} />
    </div>
  );
}

function StatCard({ label, value, icon, highlight = false }: { label: string; value: number | string; icon: React.ReactNode; highlight?: boolean }) {
  return (
    <Card className={"bg-white border-[#dadce0] " + (highlight ? "border-[#f59e0b]" : "")}>
      <CardHeader className="flex flex-row items-center justify-between pb-1 pt-4 px-4">
        <CardTitle className="text-xs font-medium text-[#5f6368]">{label}</CardTitle>
        <span className={highlight ? "text-[#f59e0b]" : "text-[#5f6368]"}>{icon}</span>
      </CardHeader>
      <CardContent className="px-4 pb-4">
        <p className={"text-2xl font-medium tabular-nums " + (highlight ? "text-[#f59e0b]" : "text-[#202124]")}>
          {value}
        </p>
      </CardContent>
    </Card>
  );
}

function DriftBanner({ models }: { models: BaselineModel[] }) {
  return (
    <div
      className="rounded-md border border-[#f59e0b] bg-[#fef3c7] p-4"
      data-testid="drift-alert"
    >
      <div className="flex items-center gap-2 text-[#92400e] font-medium text-sm mb-2">
        <AlertTriangle className="h-4 w-4 shrink-0" />
        Drift detected on {models.length} model{models.length > 1 ? "s" : ""}
      </div>
      <ul className="space-y-1 pl-6">
        {models.map((b) => (
          <li key={b.model} className="text-xs text-[#78350f] font-mono">
            {b.model} · score {b.drift_score.toFixed(2)} · p(tok) {b.p_output_tokens.toFixed(4)} · p(lat) {b.p_latency_ms.toFixed(4)}
          </li>
        ))}
      </ul>
    </div>
  );
}

function AllClear() {
  return (
    <div className="rounded-md border border-[#34a853]/40 bg-[#34a853]/10 px-4 py-3 flex items-center gap-2 text-[#137333] text-sm">
      <CheckCircle className="h-4 w-4 shrink-0" />
      All models within baseline — no drift detected
    </div>
  );
}

function ModelsCard({ baselines, projectName, projectID, onDelete }: { baselines: BaselineModel[]; projectName?: string; projectID?: string; onDelete?: () => void }) {
  return (
    <Card className="bg-white border-[#dadce0]">
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium text-[#202124]">Models</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        {baselines.length === 0 ? <EmptyState projectName={projectName} /> : <BaselineTable baselines={baselines} projectID={projectID} onDelete={onDelete} />}
      </CardContent>
    </Card>
  );
}

function BaselineTable({ baselines, projectID, onDelete }: { baselines: BaselineModel[]; projectID?: string; onDelete?: () => void }) {
  const [deleting, setDeleting] = useState<string | null>(null);

  const handleDelete = async (model: string) => {
    if (!confirm(`Delete all data for "${model}"? This cannot be undone.`)) return;
    setDeleting(model);
    try {
      await deleteModel(model, projectID || undefined);
      onDelete?.();
    } catch (e) {
      alert(`Failed to delete: ${(e as Error).message}`);
    } finally {
      setDeleting(null);
    }
  };

  return (
    <Table>
      <TableHeader>
        <TableRow className="text-xs">
          <TableHead className="pl-6">Model</TableHead>
          <TableHead className="text-right">Events</TableHead>
          <TableHead className="text-right hidden sm:table-cell">Avg Tokens</TableHead>
          <TableHead className="text-right hidden sm:table-cell">Avg Latency</TableHead>
          <TableHead className="text-center hidden md:table-cell">Baseline</TableHead>
          <TableHead className="text-center">Drift</TableHead>
          <TableHead className="text-center">Status</TableHead>
          <TableHead className="pr-6 w-8" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {baselines.map((b) => (
          <TableRow key={b.model} className={b.drift_alerted ? "bg-[#fef3c7]/40" : ""}>
            <TableCell className="pl-6 font-mono text-xs">{b.model}</TableCell>
            <TableCell className="text-right text-sm tabular-nums">{b.count.toLocaleString()}</TableCell>
            <TableCell className="text-right hidden sm:table-cell text-sm tabular-nums">
              {b.mean_output_tokens}
              <span className="text-xs text-[#5f6368] ml-1">±{b.stddev_output_tokens}</span>
            </TableCell>
            <TableCell className="text-right hidden sm:table-cell text-sm tabular-nums">
              {b.mean_latency_ms}
              <span className="text-xs text-[#5f6368] ml-1">ms ±{b.stddev_latency_ms}</span>
            </TableCell>
            <TableCell className="text-center hidden md:table-cell">
              {b.is_ready ? (
                <Badge className="bg-[#e8f0fe] text-[#1a73e8] hover:bg-[#e8f0fe] text-xs">Ready</Badge>
              ) : (
                <Badge variant="secondary" className="text-xs">{b.count}/200</Badge>
              )}
            </TableCell>
            <TableCell className="text-center">
              {b.drift_score > 0 || b.drift_alerted ? (
                <DriftBar score={b.drift_score} />
              ) : (
                <span className="text-xs text-[#5f6368]">—</span>
              )}
            </TableCell>
            <TableCell className="text-center">
              <StatusBadge b={b} />
            </TableCell>
            <TableCell className="pr-6 text-right">
              <button
                onClick={() => handleDelete(b.model)}
                disabled={deleting === b.model}
                className="inline-flex items-center justify-center w-6 h-6 rounded hover:bg-red-50 text-[#5f6368] hover:text-red-600 disabled:opacity-40 transition-colors"
                title="Delete model"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function DriftBar({ score }: { score: number }) {
  const pct = Math.round(score * 100);
  const color = score >= 0.7 ? "bg-red-500" : score >= 0.4 ? "bg-[#f59e0b]" : "bg-[#34a853]";
  return (
    <div className="flex items-center gap-2">
      <div className="w-16 h-1.5 rounded-full bg-[#e0e0e0] overflow-hidden">
        <div className={`h-full ${color}`} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs tabular-nums text-[#5f6368]">{score.toFixed(2)}</span>
    </div>
  );
}

function StatusBadge({ b }: { b: BaselineModel }) {
  if (b.drift_alerted) return <Badge className="bg-[#f59e0b] text-white hover:bg-[#f59e0b] text-xs">Drift</Badge>;
  if (b.is_ready) return <Badge className="bg-[#34a853]/10 text-[#137333] hover:bg-[#34a853]/10 text-xs">OK</Badge>;
  return <Badge variant="secondary" className="text-xs">No baseline</Badge>;
}

function EmptyState({ projectName }: { projectName?: string }) {
  return (
    <div className="py-16 text-center space-y-3">
      <div className="inline-flex items-center justify-center w-10 h-10 rounded-full bg-[#f1f3f4]">
        <Database className="h-5 w-5 text-[#5f6368]" />
      </div>
      <p className="text-sm font-medium text-[#202124]">No events yet</p>
      <p className="text-xs text-[#5f6368]">
        Instrument your app with the Argus SDK and point it to{" "}
        {projectName ? <strong>{projectName}</strong> : "this project"}.
      </p>
      <code className="inline-block text-xs bg-[#f1f3f4] rounded px-2 py-1 font-mono">
        patch(endpoint, api_key=&quot;argus_sk_…&quot;)
      </code>
    </div>
  );
}

// ─── Onboarding ───────────────────────────────────────────────────────────────

function CodeSnippet({ code }: { code: string }) {
  const [copied, setCopied] = useState(false);
  const handle = () => {
    navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <div className="relative group mt-2">
      <pre className="bg-[#f8f9fa] border border-[#e8eaed] rounded-md text-xs font-mono p-3 overflow-x-auto text-[#202124] leading-relaxed">{code}</pre>
      <button
        onClick={handle}
        className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity inline-flex items-center gap-1 bg-white border border-[#dadce0] rounded px-2 py-0.5 text-[10px] text-[#5f6368] hover:bg-[#f1f3f4]"
      >
        <Copy className="h-3 w-3" />
        {copied ? "Copied!" : "Copy"}
      </button>
    </div>
  );
}

function OnboardingStep({ number, title, children }: { number: number; title: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-4 bg-white border border-[#dadce0] rounded-lg p-5">
      <div className="shrink-0 w-7 h-7 rounded-full bg-[#1a73e8] text-white text-sm font-medium flex items-center justify-center">
        {number}
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-[#202124]">{title}</p>
        {children}
      </div>
    </div>
  );
}

function Onboarding({ onRefresh, refreshing }: { onRefresh: () => void; refreshing: boolean }) {
  return (
    <div className="max-w-2xl space-y-8">
      {/* Header */}
      <div className="flex items-start gap-4">
        <div className="shrink-0 inline-flex items-center justify-center w-10 h-10 rounded-xl bg-[#e8f0fe]">
          <Terminal className="h-5 w-5 text-[#1a73e8]" />
        </div>
        <div>
          <h2 className="text-lg font-medium text-[#202124]">Get started with Argus</h2>
          <p className="text-sm text-[#5f6368] mt-0.5">
            Instrument your LLM app in minutes. Argus captures signals from every call and detects behavioral drift automatically.
          </p>
        </div>
      </div>

      {/* Steps */}
      <div className="space-y-3">
        <OnboardingStep number={1} title="Install the SDK">
          <CodeSnippet code="pip install argus-sdk" />
          <p className="text-xs text-[#5f6368] mt-2">
            Using OpenAI?{" "}
            <code className="bg-[#f1f3f4] px-1 rounded">pip install &quot;argus-sdk[openai]&quot;</code>
          </p>
        </OnboardingStep>

        <OnboardingStep number={2} title="Add one line before your LLM client">
          <CodeSnippet code={`from argus_sdk import patch
patch(api_key="argus_sk_...")   # get your key from the CLI: argus projects

import anthropic
client = anthropic.Anthropic()  # automatically instrumented`} />
          <p className="text-xs text-[#5f6368] mt-2">
            Works the same for <code className="bg-[#f1f3f4] px-1 rounded">AsyncAnthropic</code>,{" "}
            <code className="bg-[#f1f3f4] px-1 rounded">OpenAI</code>, and{" "}
            <code className="bg-[#f1f3f4] px-1 rounded">AsyncOpenAI</code>. Streaming is supported too.
          </p>
        </OnboardingStep>

        <OnboardingStep number={3} title="Get an API key">
          <p className="text-xs text-[#5f6368] mt-1">Log in and list your projects to find your key:</p>
          <CodeSnippet code={`pip install argus-sdk
argus login     # opens browser — sign in with GitHub or Google
argus projects  # shows your project IDs and key prefixes`} />
        </OnboardingStep>

        <OnboardingStep number={4} title="Make LLM calls — Argus does the rest">
          <p className="text-xs text-[#5f6368] mt-1">
            Every response is captured automatically in the background — no changes to your existing code needed.
            Argus needs <strong>200 events per model</strong> to build a baseline, then checks for drift every 60 seconds.
          </p>
          <div className="mt-3 flex items-start gap-2 bg-[#e8f0fe] rounded-md p-3">
            <CheckCircle className="h-3.5 w-3.5 text-[#1a73e8] mt-0.5 shrink-0" />
            <p className="text-xs text-[#1a73e8]">
              No prompt text or completion text is ever sent — only derived signals (tokens, latency, finish reason).
            </p>
          </div>
        </OnboardingStep>
      </div>

      {/* Check button */}
      <div className="flex items-center gap-3">
        <button
          onClick={onRefresh}
          disabled={refreshing}
          className="inline-flex items-center gap-2 h-8 px-4 rounded-md bg-[#1a73e8] text-white text-sm hover:bg-[#1557b0] disabled:opacity-60 transition-colors"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          {refreshing ? "Checking…" : "Check for events"}
        </button>
        <span className="text-xs text-[#5f6368]">No events received yet</span>
      </div>
    </div>
  );
}

function ErrorBanner({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="rounded-md border border-red-300 bg-red-50 p-4 flex items-start gap-3">
      <AlertTriangle className="h-4 w-4 text-red-600 mt-0.5 shrink-0" />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-red-700">Failed to load data</p>
        <p className="text-xs text-red-600 mt-0.5 truncate">{message}</p>
      </div>
      <button onClick={onRetry} className="text-xs text-red-700 hover:underline shrink-0">Retry</button>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-6 animate-pulse">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        {[...Array(4)].map((_, i) => <div key={i} className="h-24 rounded-md border border-[#dadce0] bg-white" />)}
      </div>
      <div className="h-64 rounded-md border border-[#dadce0] bg-white" />
    </div>
  );
}
