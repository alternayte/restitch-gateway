import { useQuery } from "@tanstack/react-query";
import { Activity, Server } from "lucide-react";
import { Fragment, useState } from "react";
import { getUsageOptions, listInstancesOptions, listRequestsOptions } from "@/api/@tanstack/react-query.gen";
import type { SampledRequest } from "@/api/types.gen";
import { chartColors, LineChart, StackedChart, type ChartRow } from "@/components/charts";
import { DataState, EmptyState } from "@/components/data-state";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/ui/page-header";
import { SegmentedControl } from "@/components/ui/segmented";
import { TBody, THead, Table, Td, Th, Tr } from "@/components/ui/table";
import { formatMs, formatRate } from "@/lib/charts";
import { formatRelative, formatTime } from "@/lib/utils";

const ranges = [
  { value: "1h", label: "1 hour", ms: 3_600_000 },
  { value: "6h", label: "6 hours", ms: 6 * 3_600_000 },
  { value: "24h", label: "24 hours", ms: 24 * 3_600_000 },
] as const;
type Range = (typeof ranges)[number]["value"];

const minute = (iso: string) => new Date(iso).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });

/** OverviewPage shows what the environment's gateways report: instances, usage and sampled requests. */
export function OverviewPage({ env }: { env: string }) {
  const [range, setRange] = useState<Range>("1h");
  const [from] = useState(() => Date.now());
  const span = ranges.find((r) => r.value === range)!.ms;
  const query = { from: new Date(from - span).toISOString() };
  const instances = useQuery({ ...listInstancesOptions({ path: { env } }), refetchInterval: 15_000 });
  const usage = useQuery({ ...getUsageOptions({ path: { env }, query }), refetchInterval: 60_000 });
  const requests = useQuery({ ...listRequestsOptions({ path: { env }, query: { limit: 50 } }), refetchInterval: 15_000 });

  const noInstances = instances.data !== undefined && (instances.data.instances ?? []).length === 0;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={env}
        description="What the gateways of this environment report. Usage arrives once a minute, samples every 10 seconds."
        actions={
          <SegmentedControl
            label="Time range"
            value={range}
            onChange={setRange}
            options={ranges.map((r) => ({ value: r.value, label: r.label }))}
          />
        }
      />
      {noInstances ? (
        <EmptyState
          icon={Server}
          text={`No gateway has reported yet. Start one with RESTITCH_STUDIO_URL and a gateway token of ${env} in RESTITCH_GATEWAY_TOKEN, then publish a version.`}
        />
      ) : (
        <>
          <DataState query={usage} skeleton="panel">
            {(u) => <UsageSection buckets={u.buckets ?? []} upstreams={u.upstreams ?? []} />}
          </DataState>
          <section className="flex flex-col gap-3">
            <h2 className="text-base font-semibold">Instances</h2>
            <DataState query={instances}>
              {(d) => (
                <Table>
                  <THead>
                    <Tr>
                      <Th>Instance</Th>
                      <Th>Version</Th>
                      <Th>Binary</Th>
                      <Th>Last seen</Th>
                      <Th>Last pull</Th>
                    </Tr>
                  </THead>
                  <TBody>
                    {(d.instances ?? []).map((i) => (
                      <Tr key={i.instance_id}>
                        <Td className="font-mono text-xs">{i.instance_id}</Td>
                        <Td>
                          {i.applied_version || "none"}{" "}
                          {i.applied_version !== d.current_version && <Badge>behind {d.current_version}</Badge>}
                        </Td>
                        <Td className="font-mono text-xs">{i.binary_version}</Td>
                        <Td>
                          {formatRelative(i.last_seen_at)} {i.stale && <Badge>stale</Badge>}
                        </Td>
                        <Td className="text-xs">{i.last_pull_error ? <span className="text-destructive">{i.last_pull_error}</span> : "OK"}</Td>
                      </Tr>
                    ))}
                  </TBody>
                </Table>
              )}
            </DataState>
          </section>
          <section className="flex flex-col gap-3">
            <h2 className="text-base font-semibold">Sampled requests</h2>
            <DataState
              query={requests}
              empty={(d) => (d.requests ?? []).length === 0}
              emptyIcon={Activity}
              emptyText="No request sampled yet. Every failed request and a few successful ones per minute land here."
            >
              {(d) => <RequestList requests={d.requests ?? []} />}
            </DataState>
          </section>
        </>
      )}
    </div>
  );
}

function UsageSection({
  buckets,
  upstreams,
}: {
  buckets: { timestamp: string; requests: number; errors: number; p95_ms: number }[];
  upstreams: { name: string; requests: number; errors: number; breakers: Record<string, string> }[];
}) {
  const total = buckets.reduce((n, b) => n + b.requests, 0);
  const errors = buckets.reduce((n, b) => n + b.errors, 0);
  const p95 = buckets.reduce((m, b) => Math.max(m, b.p95_ms), 0);
  const rows: ChartRow[] = buckets.map((b) => ({ label: minute(b.timestamp), ok: b.requests - b.errors, errors: b.errors, p95: b.p95_ms }));

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Tile label="Requests" value={String(total)} />
        <Tile label="Error rate" value={total === 0 ? "—" : formatRate(errors / total)} />
        <Tile label="Slowest p95" value={total === 0 ? "—" : formatMs(p95)} />
      </div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <StackedChart
          title="Requests per minute"
          rows={rows}
          series={[
            { key: "ok", label: "Succeeded", color: chartColors[0]! },
            { key: "errors", label: "Failed", color: "var(--destructive)" },
          ]}
        />
        <LineChart title="p95 latency per minute" rows={rows} series={[{ key: "p95", label: "p95", color: chartColors[2]! }]} format={formatMs} />
      </div>
      {upstreams.length > 0 && (
        <Table>
          <THead>
            <Tr>
              <Th>Upstream</Th>
              <Th>Requests</Th>
              <Th>Errors</Th>
              <Th>Breakers</Th>
            </Tr>
          </THead>
          <TBody>
            {upstreams.map((u) => {
              const notClosed = Object.entries(u.breakers).filter(([, s]) => s !== "closed");
              return (
                <Tr key={u.name}>
                  <Td className="font-medium">{u.name}</Td>
                  <Td>{u.requests}</Td>
                  <Td>{u.requests === 0 ? "—" : formatRate(u.errors / u.requests)}</Td>
                  <Td>
                    {notClosed.length === 0
                      ? "closed"
                      : notClosed.map(([id, s]) => <Badge key={id}>{`${s} on ${id}`}</Badge>)}
                  </Td>
                </Tr>
              );
            })}
          </TBody>
        </Table>
      )}
    </div>
  );
}

function Tile({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1 rounded-panel border bg-panel p-4 shadow-panel">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="text-2xl font-semibold tabular-nums">{value}</span>
    </div>
  );
}

function RequestList({ requests }: { requests: SampledRequest[] }) {
  const [open, setOpen] = useState<string | null>(null);
  return (
    <Table>
      <THead>
        <Tr>
          <Th>Time</Th>
          <Th>Composition</Th>
          <Th>Request</Th>
          <Th>Status</Th>
          <Th>Duration</Th>
        </Tr>
      </THead>
      <TBody>
        {requests.map((r) => {
          const key = `${r.instance_id}/${r.id}`;
          const failed = r.status >= 500 || r.partial;
          return (
            <Fragment key={key}>
              <Tr
                tabIndex={0}
                aria-expanded={open === key}
                className="cursor-pointer"
                onClick={() => setOpen(open === key ? null : key)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    setOpen(open === key ? null : key);
                  }
                }}
              >
                <Td>{formatTime(r.time)}</Td>
                <Td>{r.composition}</Td>
                <Td className="font-mono text-xs">
                  {r.method} <span>{r.path}</span>
                </Td>
                <Td>
                  <span className={failed ? "text-destructive" : undefined}>{r.status}</span> {r.partial && <Badge>partial</Badge>}
                </Td>
                <Td>{formatMs(r.duration_ms)}</Td>
              </Tr>
              {open === key && (
                <Tr>
                  <Td colSpan={5}>
                    <Waterfall request={r} />
                  </Td>
                </Tr>
              )}
            </Fragment>
          );
        })}
      </TBody>
    </Table>
  );
}

/** Waterfall draws each step's start and duration on the request's time axis. */
function Waterfall({ request }: { request: SampledRequest }) {
  const steps = request.steps ?? [];
  const total = Math.max(request.duration_ms, ...steps.map((s) => s.start_offset_ms + s.duration_ms), 1);
  if (steps.length === 0) return <p className="text-sm text-muted-foreground">This request ran no step.</p>;
  return (
    <div className="flex flex-col gap-2 py-1">
      {steps.map((s) => (
        <div key={s.name} className="grid grid-cols-[10rem_1fr_6rem] items-center gap-3 text-xs">
          <span className="truncate font-medium" title={s.url}>
            {s.name} <span className="text-muted-foreground">{s.upstream}</span>
          </span>
          <div className="relative h-3 rounded bg-muted">
            <div
              className={s.status === "failed" ? "absolute h-3 rounded bg-destructive" : "absolute h-3 rounded bg-accent"}
              style={{ left: `${(s.start_offset_ms / total) * 100}%`, width: `${Math.max((s.duration_ms / total) * 100, 0.5)}%` }}
            />
          </div>
          <span className="text-right tabular-nums">
            {formatMs(s.duration_ms)} {s.cached && <Badge>cached</Badge>}
          </span>
          {s.error && <span className="col-span-3 font-mono break-words text-destructive">{s.error}</span>}
        </div>
      ))}
    </div>
  );
}
