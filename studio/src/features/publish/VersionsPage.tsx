import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { GitCompareArrows, History, Rocket, RotateCcw, Upload } from "lucide-react";
import { useState } from "react";
import {
  getVersionOptions,
  listEnvironmentsOptions,
  listResourcesQueryKey,
  listVersionsOptions,
  listVersionsQueryKey,
  previewPublishMutation,
  promoteMutation,
  publishMutation,
  rollbackMutation,
} from "@/api/@tanstack/react-query.gen";
import type { ArtifactVersion } from "@/api/types.gen";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DataState } from "@/components/data-state";
import { DiffView } from "@/components/diff-view";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, FormError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { PageHeader } from "@/components/ui/page-header";
import { TBody, THead, Table, Td, Th, Tr } from "@/components/ui/table";
import { toast } from "@/components/ui/toast";
import { useCurrentUser } from "@/lib/auth";
import { errorMessage } from "@/lib/errors";
import { can } from "@/lib/roles";
import { formatTime } from "@/lib/utils";

/** VersionsPage publishes the draft and lists the environment's versions. */
export function VersionsPage({ env }: { env: string }) {
  const me = useCurrentUser();
  const canEdit = can(me.role, "editor");
  const versions = useQuery(listVersionsOptions({ path: { env } }));
  const [rollbackTo, setRollbackTo] = useState<ArtifactVersion | null>(null);
  const [viewing, setViewing] = useState<number | null>(null);
  const [promoting, setPromoting] = useState(false);
  const qc = useQueryClient();
  const rollback = useMutation({
    ...rollbackMutation(),
    onSuccess: (v) => {
      void qc.invalidateQueries({ queryKey: listVersionsQueryKey({ path: { env } }) });
      toast({ title: `Published version ${v.number}`, description: v.message });
      setRollbackTo(null);
    },
  });
  const list = versions.data?.versions ?? [];
  const current = list[0]?.number ?? 0;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={`Publish ${env}`}
        description="Publishing validates the whole draft and writes a new version. Gateways pull the newest version."
        actions={
          canEdit && (
            <Button variant="secondary" onClick={() => setPromoting(true)}>
              <GitCompareArrows className="h-4 w-4" aria-hidden />
              Promote from another environment
            </Button>
          )
        }
      />
      {canEdit && <PublishPanel env={env} />}
      <section className="flex flex-col gap-3">
        <h2 className="text-base font-semibold">History</h2>
        <DataState
          query={versions}
          empty={(d) => (d.versions ?? []).length === 0}
          emptyIcon={History}
          emptyText="Nothing published yet."
        >
          {() => (
            <Table>
              <THead>
                <Tr>
                  <Th>Version</Th>
                  <Th>Message</Th>
                  <Th>Author</Th>
                  <Th>Published</Th>
                  <Th className="text-right">Actions</Th>
                </Tr>
              </THead>
              <TBody>
                {list.map((v) => (
                  <Tr key={v.number}>
                    <Td className="font-medium">
                      {v.number} {v.number === current && <Badge>current</Badge>}
                    </Td>
                    <Td>{v.message}</Td>
                    <Td>{v.author}</Td>
                    <Td>{formatTime(v.created_at)}</Td>
                    <Td className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button size="sm" variant="ghost" onClick={() => setViewing(v.number)}>
                          View
                        </Button>
                        {canEdit && v.number !== current && (
                          <Button size="sm" variant="ghost" onClick={() => setRollbackTo(v)}>
                            <RotateCcw className="h-3.5 w-3.5" aria-hidden />
                            Roll back
                          </Button>
                        )}
                      </div>
                    </Td>
                  </Tr>
                ))}
              </TBody>
            </Table>
          )}
        </DataState>
      </section>
      <ConfirmDialog
        open={rollbackTo !== null}
        onClose={() => setRollbackTo(null)}
        title={`Roll back ${env} to version ${rollbackTo?.number ?? ""}?`}
        confirmLabel="Roll back"
        pending={rollback.isPending}
        error={rollback.isError ? errorMessage(rollback.error) : undefined}
        onConfirm={() => rollbackTo && rollback.mutate({ path: { env, number: rollbackTo.number } })}
      >
        Version {rollbackTo?.number} is published again as version {current + 1}. History stays as it is, and the draft does
        not change. Gateways pull it within one poll interval.
      </ConfirmDialog>
      {viewing !== null && <VersionDialog env={env} number={viewing} onClose={() => setViewing(null)} />}
      <PromoteDialog env={env} open={promoting} onClose={() => setPromoting(false)} />
    </div>
  );
}

function PublishPanel({ env }: { env: string }) {
  const qc = useQueryClient();
  const [message, setMessage] = useState("");
  const preview = useMutation(previewPublishMutation());
  const publish = useMutation({
    ...publishMutation(),
    onSuccess: (v) => {
      void qc.invalidateQueries({ queryKey: listVersionsQueryKey({ path: { env } }) });
      toast({ title: `Published version ${v.number}`, description: v.message });
      setMessage("");
      preview.reset();
    },
  });
  const pv = preview.data;
  const unchanged = pv?.valid && pv.diff === "";

  return (
    <section className="flex flex-col gap-4 rounded-panel border bg-panel p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-col">
          <h2 className="text-base font-semibold">Draft</h2>
          <p className="text-sm text-muted-foreground">Review the changes since the current version, then publish them.</p>
        </div>
        <Button variant="secondary" onClick={() => preview.mutate({ path: { env } })} disabled={preview.isPending}>
          <Upload className="h-4 w-4" aria-hidden />
          {preview.isPending ? "Checking" : "Review changes"}
        </Button>
      </div>
      {preview.isError && <FormError>{errorMessage(preview.error)}</FormError>}
      {pv && !pv.valid && (
        <div role="alert" className="flex flex-col gap-1 rounded-control border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
          <span className="font-medium">The draft does not validate. Fix it before you publish.</span>
          {(pv.errors ?? []).map((e) => (
            <span key={e} className="font-mono text-xs break-words">
              {e}
            </span>
          ))}
        </div>
      )}
      {pv?.valid && (unchanged ? (
        <p className="text-sm text-muted-foreground">The draft matches version {pv.current_version}. There is nothing to publish.</p>
      ) : (
        <DiffView text={pv.diff} label="Changes to publish" />
      ))}
      {pv && (
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            publish.mutate({ path: { env }, body: { message } });
          }}
        >
          <div className="min-w-64 flex-1">
            <Field id="publish-message" label="Message" hint="Say what changed and why. It appears in the history.">
              <Input value={message} onChange={(e) => setMessage(e.target.value)} />
            </Field>
          </div>
          <Button type="submit" disabled={!pv.valid || unchanged || !message.trim() || publish.isPending}>
            <Rocket className="h-4 w-4" aria-hidden />
            {publish.isPending ? "Publishing" : "Publish"}
          </Button>
        </form>
      )}
      {publish.isError && <FormError>{errorMessage(publish.error)}</FormError>}
    </section>
  );
}

function VersionDialog({ env, number, onClose }: { env: string; number: number; onClose: () => void }) {
  const query = useQuery(getVersionOptions({ path: { env, number } }));
  return (
    <Dialog open onClose={onClose} title={`${env} version ${number}`} size="lg">
      <DataState query={query} skeleton="lines">
        {(v) => <pre className="max-h-[60vh] overflow-auto rounded-control border bg-panel p-3 font-mono text-xs">{v.yaml}</pre>}
      </DataState>
    </Dialog>
  );
}

function PromoteDialog({ env, open, onClose }: { env: string; open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const envs = useQuery({ ...listEnvironmentsOptions(), enabled: open });
  const others = (envs.data?.environments ?? []).filter((e) => e.name !== env);
  const [from, setFrom] = useState("");
  const source = from || others[0]?.name || "";
  const versions = useQuery({ ...listVersionsOptions({ path: { env: source } }), enabled: open && source !== "" });
  const [version, setVersion] = useState(0);
  const chosen = version || versions.data?.versions?.[0]?.number || 0;
  const [diff, setDiff] = useState<string | null>(null);
  const promote = useMutation({
    ...promoteMutation(),
    onSuccess: (out) => {
      void qc.invalidateQueries({ queryKey: listResourcesQueryKey({ path: { env } }) });
      setDiff(out.diff);
      toast({ title: "Draft replaced", description: `${env} now holds ${source} version ${chosen}. Publish to apply it.` });
    },
  });
  const close = () => {
    setDiff(null);
    promote.reset();
    onClose();
  };

  return (
    <Dialog open={open} onClose={close} title={`Promote into the ${env} draft`} size="lg">
      {diff !== null ? (
        <div className="flex flex-col gap-3">
          {diff === "" ? <p className="text-sm">The draft already matched.</p> : <DiffView text={diff} label="Draft changes" />}
          <Button className="self-end" onClick={close}>
            Done
          </Button>
        </div>
      ) : others.length === 0 ? (
        <p className="text-sm text-muted-foreground">There is no other environment to promote from.</p>
      ) : (
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            promote.mutate({ path: { env }, body: { from_environment: source, version: chosen } });
          }}
        >
          <p className="text-sm text-muted-foreground">
            The chosen version replaces the whole {env} draft. Nothing is published until you publish.
          </p>
          <Field id="promote-from" label="From environment">
            <select className="h-8 rounded-control border bg-panel px-2 text-sm" value={source} onChange={(e) => setFrom(e.target.value)}>
              {others.map((e) => (
                <option key={e.name}>{e.name}</option>
              ))}
            </select>
          </Field>
          <Field id="promote-version" label="Version">
            <select
              className="h-8 rounded-control border bg-panel px-2 text-sm"
              value={chosen}
              onChange={(e) => setVersion(Number(e.target.value))}
            >
              {(versions.data?.versions ?? []).map((v) => (
                <option key={v.number} value={v.number}>
                  {v.number}: {v.message}
                </option>
              ))}
            </select>
          </Field>
          {promote.isError && <FormError>{errorMessage(promote.error)}</FormError>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" disabled={chosen === 0 || promote.isPending}>
              Replace the draft
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
