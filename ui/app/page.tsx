import { fetchBaselines } from "@/lib/api";
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
import { Activity, AlertTriangle, CheckCircle, Database, RefreshCw } from "lucide-react";
import type { BaselineModel } from "@/lib/types";
import { RefreshButton } from "@/components/refresh-button";

export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  let data;
  let fetchError: string | null = null;

  try {
    data = await fetchBaselines();
  } catch (e) {
    fetchError = e instanceof Error ? e.message : "Failed to connect to Argus server";
  }

  const alertedModels = data?.baselines.filter((b) => b.drift_alerted) ?? [];
  const checkedModels = data?.baselines.filter((b) => b.drift_score > 0 || b.drift_alerted) ?? [];

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b">
        <div className="max-w-6xl mx-auto px-6 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Activity className="h-6 w-6 text-primary" />
            <h1 className="text-xl font-semibold tracking-tight">Argus</h1>
            <span className="text-sm text-muted-foreground">LLM Drift Monitor</span>
          </div>
          <RefreshButton />
        </div>
      </header>

      <main className="max-w-6xl mx-auto px-6 py-8 space-y-6">
        {fetchError ? (
          <ErrorBanner message={fetchError} />
        ) : data ? (
          <>
            {/* Active alerts — shown prominently at the top */}
            {alertedModels.length > 0 && (
              <div className="rounded-lg border border-yellow-400/60 bg-yellow-50 dark:bg-yellow-950/20 p-4 space-y-2">
                <div className="flex items-center gap-2 text-yellow-700 dark:text-yellow-400 font-semibold">
                  <AlertTriangle className="h-5 w-5" />
                  Drift detected on {alertedModels.length} model{alertedModels.length > 1 ? "s" : ""}
                </div>
                <ul className="space-y-1">
                  {alertedModels.map((b) => (
                    <li key={b.model} className="text-sm text-yellow-800 dark:text-yellow-300 font-mono">
                      {b.model} — score {b.drift_score.toFixed(2)}
                      {" · "}p(tokens) {b.p_output_tokens.toFixed(4)}
                      {" · "}p(latency) {b.p_latency_ms.toFixed(4)}
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {/* All clear */}
            {checkedModels.length > 0 && alertedModels.length === 0 && (
              <div className="rounded-lg border border-green-400/60 bg-green-50 dark:bg-green-950/20 p-3 flex items-center gap-2 text-green-700 dark:text-green-400 text-sm">
                <CheckCircle className="h-4 w-4" />
                All models within baseline — no drift detected
              </div>
            )}

            {/* Summary cards */}
            <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
              <SummaryCard
                title="Total Events"
                value={data.total_events.toLocaleString()}
                icon={<Database className="h-4 w-4 text-muted-foreground" />}
              />
              <SummaryCard
                title="Models Tracked"
                value={data.baselines.length}
                icon={<Activity className="h-4 w-4 text-muted-foreground" />}
              />
              <SummaryCard
                title="Baselines Ready"
                value={data.baselines.filter((b) => b.is_ready).length}
                icon={<RefreshCw className="h-4 w-4 text-muted-foreground" />}
              />
              <SummaryCard
                title="Alerts Active"
                value={alertedModels.length}
                icon={<AlertTriangle className={`h-4 w-4 ${alertedModels.length > 0 ? "text-yellow-500" : "text-muted-foreground"}`} />}
              />
            </div>

            {/* Baselines + drift table */}
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Models</CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                {data.baselines.length === 0 ? (
                  <EmptyState />
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

function SummaryCard({ title, value, icon }: { title: string; value: number | string; icon: React.ReactNode }) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{title}</CardTitle>
        {icon}
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-bold">{value}</p>
      </CardContent>
    </Card>
  );
}

function DriftScoreBar({ score }: { score: number }) {
  const pct = Math.round(score * 100);
  const color =
    score >= 0.7 ? "bg-yellow-500" :
    score >= 0.4 ? "bg-orange-400" :
    "bg-green-500";
  return (
    <div className="flex items-center gap-2">
      <div className="w-20 h-2 rounded-full bg-muted overflow-hidden">
        <div className={`h-full rounded-full ${color}`} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs tabular-nums">{score.toFixed(2)}</span>
    </div>
  );
}

function BaselineTable({ baselines }: { baselines: BaselineModel[] }) {
  const hasAnyDriftData = baselines.some((b) => b.drift_score > 0 || b.drift_alerted);

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Model</TableHead>
          <TableHead className="text-right">Events</TableHead>
          <TableHead className="text-right">Mean Tokens</TableHead>
          <TableHead className="text-right">Mean Latency</TableHead>
          <TableHead className="text-center">Baseline</TableHead>
          {hasAnyDriftData && <TableHead className="text-center">Drift Score</TableHead>}
          <TableHead className="text-center">Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {baselines.map((b) => (
          <TableRow key={b.model} className={b.drift_alerted ? "bg-yellow-50/50 dark:bg-yellow-950/10" : ""}>
            <TableCell className="font-mono text-sm">{b.model}</TableCell>
            <TableCell className="text-right">{b.count.toLocaleString()}</TableCell>
            <TableCell className="text-right">
              {b.mean_output_tokens}
              <span className="text-muted-foreground text-xs"> ±{b.stddev_output_tokens}</span>
            </TableCell>
            <TableCell className="text-right">
              {b.mean_latency_ms} ms
              <span className="text-muted-foreground text-xs"> ±{b.stddev_latency_ms}</span>
            </TableCell>
            <TableCell className="text-center">
              {b.is_ready ? (
                <Badge variant="default">Ready</Badge>
              ) : (
                <Badge variant="secondary">{b.count}/200</Badge>
              )}
            </TableCell>
            {hasAnyDriftData && (
              <TableCell className="text-center">
                {b.drift_score > 0 || b.drift_alerted ? (
                  <DriftScoreBar score={b.drift_score} />
                ) : (
                  <span className="text-xs text-muted-foreground">—</span>
                )}
              </TableCell>
            )}
            <TableCell className="text-center">
              {b.drift_alerted ? (
                <Badge variant="destructive" className="bg-yellow-500 hover:bg-yellow-500 text-white">
                  ⚠ Drift
                </Badge>
              ) : b.is_ready && (b.drift_score > 0) ? (
                <Badge variant="outline" className="text-green-600 border-green-400">
                  OK
                </Badge>
              ) : b.is_ready ? (
                <Badge variant="outline">Monitoring</Badge>
              ) : (
                <Badge variant="secondary">Warming up</Badge>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function EmptyState() {
  return (
    <div className="py-16 text-center text-muted-foreground space-y-2">
      <p className="text-sm">No events received yet.</p>
      <p className="text-xs">
        Run{" "}
        <code className="font-mono bg-muted px-1 py-0.5 rounded text-xs">
          python examples/demo-app/simulate.py
        </code>{" "}
        to populate with test data.
      </p>
    </div>
  );
}

function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
      <strong>Could not reach Argus server:</strong> {message}
      <p className="mt-1 text-xs text-muted-foreground">
        Make sure the server is running:{" "}
        <code className="font-mono">cd server && go run ./cmd/main.go</code>
      </p>
    </div>
  );
}
