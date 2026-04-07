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
import { Activity, Database, RefreshCw } from "lucide-react";
import type { BaselineModel } from "@/lib/types";
import { RefreshButton } from "@/components/refresh-button";

export const dynamic = "force-dynamic"; // always fetch fresh on page load

export default async function DashboardPage() {
  let data;
  let fetchError: string | null = null;

  try {
    data = await fetchBaselines();
  } catch (e) {
    fetchError = e instanceof Error ? e.message : "Failed to connect to Argus server";
  }

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
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

      <main className="max-w-6xl mx-auto px-6 py-8 space-y-8">
        {fetchError ? (
          <ErrorBanner message={fetchError} />
        ) : data ? (
          <>
            {/* Summary cards */}
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
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
            </div>

            {/* Baselines table */}
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Model Baselines</CardTitle>
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

function SummaryCard({
  title,
  value,
  icon,
}: {
  title: string;
  value: number | string;
  icon: React.ReactNode;
}) {
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

function BaselineTable({ baselines }: { baselines: BaselineModel[] }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Model</TableHead>
          <TableHead className="text-right">Events</TableHead>
          <TableHead className="text-right">Mean Tokens</TableHead>
          <TableHead className="text-right">±Stddev</TableHead>
          <TableHead className="text-right">Mean Latency</TableHead>
          <TableHead className="text-right">±Stddev</TableHead>
          <TableHead className="text-center">Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {baselines.map((b) => (
          <TableRow key={b.model}>
            <TableCell className="font-mono text-sm">{b.model}</TableCell>
            <TableCell className="text-right">{b.count.toLocaleString()}</TableCell>
            <TableCell className="text-right">{b.mean_output_tokens}</TableCell>
            <TableCell className="text-right text-muted-foreground">
              ±{b.stddev_output_tokens}
            </TableCell>
            <TableCell className="text-right">{b.mean_latency_ms} ms</TableCell>
            <TableCell className="text-right text-muted-foreground">
              ±{b.stddev_latency_ms} ms
            </TableCell>
            <TableCell className="text-center">
              {b.is_ready ? (
                <Badge variant="default">Ready</Badge>
              ) : (
                <Badge variant="secondary">{b.count}/200</Badge>
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
        Install the SDK and call{" "}
        <code className="font-mono bg-muted px-1 py-0.5 rounded text-xs">patch()</code> to start
        tracking.
      </p>
    </div>
  );
}

function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
      <strong>Could not reach Argus server:</strong> {message}
      <p className="mt-1 text-xs text-muted-foreground">
        Make sure the server is running at{" "}
        <code className="font-mono">http://localhost:4000</code>
      </p>
    </div>
  );
}
