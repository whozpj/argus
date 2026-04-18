"use client";

import { useEffect, useState, useCallback, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import {
  Activity,
  AlertTriangle,
  CheckCircle,
  Database,
  LogOut,
  RefreshCw,
  Zap,
} from "lucide-react";

import { fetchMe, fetchBaselines, logout } from "@/lib/api";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { BaselinesResponse, MeResponse, Project } from "@/lib/types";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select";
import type { BaselineModel } from "@/lib/types";
import { Settings } from "lucide-react";

// ─── Main dashboard ────────────────────────────────────────────────────────────

function DashboardInner() {
  const router = useRouter();
  const params = useSearchParams();
  const selectedProject = params.get("project") ?? "";

  const [me, setMe] = useState<MeResponse | null>(null);
  const [data, setData] = useState<BaselinesResponse | null>(null);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  // Auth check — redirect to /login if no valid token.
  useEffect(() => {
    fetchMe()
      .then((m) => {
        setMe(m);
        // Auto-select first project if none in URL.
        if (!params.get("project") && m.projects.length > 0) {
          const url = new URL(window.location.href);
          url.searchParams.set("project", m.projects[0].id);
          router.replace(url.pathname + url.search);
        }
      })
      .catch(() => {
        router.replace("/login");
      });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Fetch baselines whenever the selected project changes.
  const loadBaselines = useCallback(
    (isRefresh = false) => {
      if (isRefresh) setRefreshing(true);
      else setLoading(true);
      setFetchError(null);

      fetchBaselines(selectedProject || undefined)
        .then((d) => {
          setData(d);
        })
        .catch((e: Error) => setFetchError(e.message))
        .finally(() => {
          setLoading(false);
          setRefreshing(false);
        });
    },
    [selectedProject],
  );

  useEffect(() => {
    if (me) loadBaselines();
  }, [me, loadBaselines]);

  const handleProjectChange = (id: string | null) => {
    if (!id) return;
    const url = new URL(window.location.href);
    url.searchParams.set("project", id);
    router.push(url.pathname + url.search);
  };

  const handleLogout = () => {
    logout();
    router.replace("/login");
  };

  const alertedModels = data?.baselines.filter((b) => b.drift_alerted) ?? [];
  const checkedModels =
    data?.baselines.filter((b) => b.drift_score > 0 || b.drift_alerted) ?? [];
  const currentProject = me?.projects.find((p) => p.id === selectedProject);

  return (
    <div className="min-h-screen bg-background" data-testid="dashboard">
      {/* ── Header ──────────────────────────────────────────────────────────── */}
      <header className="sticky top-0 z-10 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="max-w-6xl mx-auto px-6 h-14 flex items-center justify-between gap-4">
          {/* Brand */}
          <div className="flex items-center gap-2.5 shrink-0">
            <div className="flex items-center justify-center w-7 h-7 rounded-md bg-primary text-primary-foreground">
              <Activity className="h-4 w-4" />
            </div>
            <span className="font-semibold tracking-tight text-sm">Argus</span>
            <span className="hidden sm:block text-muted-foreground text-sm">
              / LLM Drift Monitor
            </span>
          </div>

          {/* Right controls */}
          <div className="flex items-center gap-2">
            {/* Project selector */}
            {me && me.projects.length > 0 && (
              <Select
                value={selectedProject}
                onValueChange={handleProjectChange}
              >
                <SelectTrigger
                  className="h-8 text-sm w-44"
                  data-testid="project-selector"
                >
                  {/* Render the name directly — Base UI SelectValue shows the raw value (ID) */}
                  <span className="flex-1 text-left truncate">
                    {me.projects.find((p) => p.id === selectedProject)?.name ?? "Select project"}
                  </span>
                </SelectTrigger>
                <SelectContent>
                  {me.projects.map((p: Project) => (
                    <SelectItem key={p.id} value={p.id}>
                      {p.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}

            {/* Refresh */}
            <button
              onClick={() => loadBaselines(true)}
              disabled={refreshing}
              aria-label="Refresh"
              className="inline-flex items-center justify-center h-8 w-8 rounded-md border border-input bg-background text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
            >
              <RefreshCw
                className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`}
              />
            </button>

            {/* User dropdown */}
            {me && (
              <div className="pl-2 border-l">
                <DropdownMenu>
                  <DropdownMenuTrigger
                    data-testid="user-email"
                    className="text-xs text-muted-foreground hover:text-foreground transition-colors max-w-40 truncate hidden md:block"
                    title={me.email}
                  >
                    {me.display_name ?? me.email}
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-44">
                    <DropdownMenuItem
                      onClick={() => router.push("/settings")}
                      className="flex items-center gap-2 cursor-pointer"
                    >
                      <Settings className="h-3.5 w-3.5" />
                      Settings
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      data-testid="sign-out"
                      onClick={handleLogout}
                      className="flex items-center gap-2 cursor-pointer text-muted-foreground"
                    >
                      <LogOut className="h-3.5 w-3.5" />
                      Sign out
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            )}
          </div>
        </div>
      </header>

      {/* ── Main content ────────────────────────────────────────────────────── */}
      <main className="max-w-6xl mx-auto px-6 py-8 space-y-6">
        {loading ? (
          <LoadingSkeleton />
        ) : fetchError ? (
          <ErrorBanner message={fetchError} onRetry={() => loadBaselines()} />
        ) : data ? (
          <>
            {/* Project heading */}
            {currentProject && (
              <div>
                <h2 className="text-base font-semibold">{currentProject.name}</h2>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Project ID:{" "}
                  <code className="font-mono">{currentProject.id}</code>
                </p>
              </div>
            )}

            {/* Drift alert banner */}
            {alertedModels.length > 0 && (
              <div
                className="rounded-xl border border-amber-300 bg-amber-50 dark:border-amber-700/50 dark:bg-amber-950/20 p-4"
                data-testid="drift-alert"
              >
                <div className="flex items-center gap-2 text-amber-700 dark:text-amber-400 font-semibold text-sm mb-2">
                  <AlertTriangle className="h-4 w-4 shrink-0" />
                  Drift detected on {alertedModels.length} model
                  {alertedModels.length > 1 ? "s" : ""}
                </div>
                <ul className="space-y-1">
                  {alertedModels.map((b) => (
                    <li
                      key={b.model}
                      className="text-xs text-amber-800 dark:text-amber-300 font-mono pl-6"
                    >
                      {b.model}
                      <span className="text-amber-600">
                        {" "}· score {b.drift_score.toFixed(2)}
                      </span>
                      <span className="text-amber-500">
                        {" "}· p(tok) {b.p_output_tokens.toFixed(4)} · p(lat){" "}
                        {b.p_latency_ms.toFixed(4)}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {/* All-clear banner */}
            {checkedModels.length > 0 && alertedModels.length === 0 && (
              <div className="rounded-xl border border-emerald-300 bg-emerald-50 dark:border-emerald-700/50 dark:bg-emerald-950/20 px-4 py-3 flex items-center gap-2 text-emerald-700 dark:text-emerald-400 text-sm">
                <CheckCircle className="h-4 w-4 shrink-0" />
                All models within baseline — no drift detected
              </div>
            )}

            {/* Summary cards */}
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
              <StatCard
                label="Total Events"
                value={data.total_events.toLocaleString()}
                icon={<Database className="h-4 w-4" />}
              />
              <StatCard
                label="Models"
                value={data.baselines.length}
                icon={<Zap className="h-4 w-4" />}
              />
              <StatCard
                label="Baselines Ready"
                value={data.baselines.filter((b) => b.is_ready).length}
                icon={<CheckCircle className="h-4 w-4" />}
              />
              <StatCard
                label="Alerts"
                value={alertedModels.length}
                icon={<AlertTriangle className="h-4 w-4" />}
                highlight={alertedModels.length > 0}
              />
            </div>

            {/* Models table */}
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-semibold">Models</CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                {data.baselines.length === 0 ? (
                  <EmptyState projectName={currentProject?.name} />
                ) : (
                  <BaselineTable baselines={data.baselines} />
                )}
              </CardContent>
            </Card>
          </>
        ) : null}
      </main>
    </div>
  );
}

export default function DashboardPage() {
  return (
    <Suspense>
      <DashboardInner />
    </Suspense>
  );
}

// ─── Sub-components ────────────────────────────────────────────────────────────

function StatCard({
  label,
  value,
  icon,
  highlight = false,
}: {
  label: string;
  value: number | string;
  icon: React.ReactNode;
  highlight?: boolean;
}) {
  return (
    <Card
      className={highlight ? "border-amber-300 dark:border-amber-700/50" : ""}
    >
      <CardHeader className="flex flex-row items-center justify-between pb-1 pt-4 px-4">
        <CardTitle className="text-xs font-medium text-muted-foreground">
          {label}
        </CardTitle>
        <span
          className={
            highlight ? "text-amber-500" : "text-muted-foreground"
          }
        >
          {icon}
        </span>
      </CardHeader>
      <CardContent className="px-4 pb-4">
        <p
          className={`text-2xl font-bold tabular-nums ${
            highlight ? "text-amber-600 dark:text-amber-400" : ""
          }`}
        >
          {value}
        </p>
      </CardContent>
    </Card>
  );
}

function DriftBar({ score }: { score: number }) {
  const pct = Math.round(score * 100);
  const color =
    score >= 0.7
      ? "bg-amber-500"
      : score >= 0.4
        ? "bg-orange-400"
        : "bg-emerald-500";
  return (
    <div className="flex items-center gap-2">
      <div className="w-16 h-1.5 rounded-full bg-muted overflow-hidden">
        <div
          className={`h-full rounded-full ${color}`}
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="text-xs tabular-nums text-muted-foreground">
        {score.toFixed(2)}
      </span>
    </div>
  );
}

function StatusBadge({ b }: { b: BaselineModel }) {
  if (b.drift_alerted)
    return (
      <Badge className="bg-amber-500 text-white hover:bg-amber-500 text-xs">
        Drift
      </Badge>
    );
  if (b.is_ready && b.drift_score > 0)
    return (
      <Badge
        variant="outline"
        className="text-emerald-600 border-emerald-400 text-xs"
      >
        OK
      </Badge>
    );
  if (b.is_ready)
    return (
      <Badge variant="outline" className="text-xs">
        Monitoring
      </Badge>
    );
  return (
    <Badge variant="secondary" className="text-xs">
      {b.count}/200
    </Badge>
  );
}

function BaselineTable({ baselines }: { baselines: BaselineModel[] }) {
  const hasDrift = baselines.some((b) => b.drift_score > 0 || b.drift_alerted);

  return (
    <Table>
      <TableHeader>
        <TableRow className="text-xs">
          <TableHead className="pl-6">Model</TableHead>
          <TableHead className="text-right">Events</TableHead>
          <TableHead className="text-right hidden sm:table-cell">
            Avg Tokens
          </TableHead>
          <TableHead className="text-right hidden sm:table-cell">
            Avg Latency
          </TableHead>
          <TableHead className="text-center hidden md:table-cell">
            Baseline
          </TableHead>
          {hasDrift && <TableHead className="text-center">Drift</TableHead>}
          <TableHead className="text-center pr-6">Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {baselines.map((b) => (
          <TableRow
            key={b.model}
            className={
              b.drift_alerted
                ? "bg-amber-50/40 dark:bg-amber-950/10"
                : ""
            }
          >
            <TableCell className="pl-6 font-mono text-xs">{b.model}</TableCell>
            <TableCell className="text-right text-sm tabular-nums">
              {b.count.toLocaleString()}
            </TableCell>
            <TableCell className="text-right hidden sm:table-cell">
              <span className="text-sm tabular-nums">
                {b.mean_output_tokens}
              </span>
              <span className="text-xs text-muted-foreground ml-1">
                ±{b.stddev_output_tokens}
              </span>
            </TableCell>
            <TableCell className="text-right hidden sm:table-cell">
              <span className="text-sm tabular-nums">{b.mean_latency_ms}</span>
              <span className="text-xs text-muted-foreground ml-1">
                ms ±{b.stddev_latency_ms}
              </span>
            </TableCell>
            <TableCell className="text-center hidden md:table-cell">
              {b.is_ready ? (
                <Badge variant="default" className="text-xs">
                  Ready
                </Badge>
              ) : (
                <Badge variant="secondary" className="text-xs">
                  {b.count}/200
                </Badge>
              )}
            </TableCell>
            {hasDrift && (
              <TableCell className="text-center">
                {b.drift_score > 0 || b.drift_alerted ? (
                  <DriftBar score={b.drift_score} />
                ) : (
                  <span className="text-xs text-muted-foreground">—</span>
                )}
              </TableCell>
            )}
            <TableCell className="text-center pr-6">
              <StatusBadge b={b} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function EmptyState({ projectName }: { projectName?: string }) {
  return (
    <div className="py-16 text-center space-y-3">
      <div className="inline-flex items-center justify-center w-10 h-10 rounded-full bg-muted">
        <Database className="h-5 w-5 text-muted-foreground" />
      </div>
      <div>
        <p className="text-sm font-medium">No events yet</p>
        <p className="text-xs text-muted-foreground mt-1">
          Instrument your app with the Argus SDK and point it to{" "}
          {projectName ? (
            <strong>{projectName}</strong>
          ) : (
            "this project"
          )}
          .
        </p>
      </div>
      <div className="inline-block">
        <code className="text-xs bg-muted rounded px-2 py-1 font-mono">
          patch(endpoint, api_key=&quot;argus_sk_…&quot;)
        </code>
      </div>
    </div>
  );
}

function ErrorBanner({
  message,
  onRetry,
}: {
  message: string;
  onRetry: () => void;
}) {
  return (
    <div className="rounded-xl border border-destructive/40 bg-destructive/5 p-4 flex items-start gap-3">
      <AlertTriangle className="h-4 w-4 text-destructive mt-0.5 shrink-0" />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-destructive">
          Failed to load data
        </p>
        <p className="text-xs text-muted-foreground mt-0.5 truncate">
          {message}
        </p>
      </div>
      <button
        onClick={onRetry}
        className="text-xs text-muted-foreground hover:text-foreground transition-colors shrink-0"
      >
        Retry
      </button>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-6 animate-pulse">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        {[...Array(4)].map((_, i) => (
          <div key={i} className="h-24 rounded-xl border bg-muted/40" />
        ))}
      </div>
      <div className="h-64 rounded-xl border bg-muted/40" />
    </div>
  );
}
