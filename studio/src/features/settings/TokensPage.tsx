import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, KeyRound, Plus } from "lucide-react";
import { useState, type FormEvent } from "react";
import {
  issueGatewayTokenMutation,
  listGatewayTokensOptions,
  listGatewayTokensQueryKey,
  revokeGatewayTokenMutation,
} from "@/api/@tanstack/react-query.gen";
import type { GatewayToken } from "@/api/types.gen";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DataState, EmptyState } from "@/components/data-state";
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
import { formatRelative, formatTime } from "@/lib/utils";

/** TokensPage manages the gateway tokens of one environment. Only owners see it. */
export function TokensPage({ env }: { env: string }) {
  const me = useCurrentUser();
  if (!can(me.role, "owner")) {
    return (
      <div className="flex flex-col gap-5">
        <PageHeader title={`Gateway tokens of ${env}`} />
        <EmptyState icon={KeyRound} text="Only owners manage gateway tokens." />
      </div>
    );
  }
  return <OwnerTokens env={env} />;
}

function OwnerTokens({ env }: { env: string }) {
  const qc = useQueryClient();
  const query = useQuery(listGatewayTokensOptions({ path: { env } }));
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<GatewayToken | null>(null);
  const revoke = useMutation({
    ...revokeGatewayTokenMutation(),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: listGatewayTokensQueryKey({ path: { env } }) });
      toast({ title: "Token revoked", description: "Gateways that use it stop receiving versions." });
      setRevoking(null);
    },
  });
  const create = (
    <Button onClick={() => setCreating(true)}>
      <Plus className="h-4 w-4" aria-hidden />
      New token
    </Button>
  );

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title={`Gateway tokens of ${env}`}
        description="A gateway sets RESTITCH_STUDIO_URL and RESTITCH_GATEWAY_TOKEN to pull this environment. A token can only read versions and report telemetry."
        actions={create}
      />
      <DataState
        query={query}
        empty={(d) => (d.tokens ?? []).length === 0}
        emptyIcon={KeyRound}
        emptyText="No token yet. Create one for each gateway deployment."
        emptyAction={create}
      >
        {(d) => (
          <Table>
            <THead>
              <Tr>
                <Th>Name</Th>
                <Th>Token</Th>
                <Th>Created</Th>
                <Th>Last used</Th>
                <Th className="text-right">Actions</Th>
              </Tr>
            </THead>
            <TBody>
              {(d.tokens ?? []).map((t) => (
                <Tr key={t.id}>
                  <Td className="font-medium">{t.name}</Td>
                  <Td className="font-mono text-xs">{t.prefix}…</Td>
                  <Td>
                    {formatTime(t.created_at)} by {t.created_by}
                  </Td>
                  <Td>{t.last_used_at ? formatRelative(t.last_used_at) : "Never"}</Td>
                  <Td className="text-right">
                    {t.revoked_at ? (
                      <Badge>revoked</Badge>
                    ) : (
                      <Button size="sm" variant="ghost" onClick={() => setRevoking(t)}>
                        Revoke
                      </Button>
                    )}
                  </Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </DataState>
      <NewTokenDialog env={env} open={creating} onClose={() => setCreating(false)} />
      <ConfirmDialog
        open={revoking !== null}
        onClose={() => setRevoking(null)}
        title={`Revoke ${revoking?.name ?? ""}?`}
        confirmLabel="Revoke"
        destructive
        pending={revoke.isPending}
        error={revoke.isError ? errorMessage(revoke.error) : undefined}
        onConfirm={() => revoking && revoke.mutate({ path: { env, id: revoking.id } })}
      >
        Gateways that use this token get 401 on their next pull and keep serving their last version until they get a new token.
      </ConfirmDialog>
    </div>
  );
}

function NewTokenDialog({ env, open, onClose }: { env: string; open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [token, setToken] = useState("");
  const issue = useMutation({
    ...issueGatewayTokenMutation(),
    onSuccess: (out) => {
      void qc.invalidateQueries({ queryKey: listGatewayTokensQueryKey({ path: { env } }) });
      setToken(out.token);
    },
  });
  const close = () => {
    setToken("");
    setName("");
    issue.reset();
    onClose();
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    issue.mutate({ path: { env }, body: { name: name.trim() } });
  };

  return (
    <Dialog open={open} onClose={close} title={token ? "Copy the token now" : "New gateway token"}>
      {token ? (
        <div className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">
            Studio keeps only a digest of the token. This is the only time it is shown.
          </p>
          <div className="flex items-center gap-2">
            <Input readOnly value={token} aria-label="Gateway token" className="font-mono text-xs" onFocus={(e) => e.target.select()} />
            <Button
              variant="secondary"
              aria-label="Copy the token"
              onClick={() => {
                void navigator.clipboard?.writeText(token);
                toast({ title: "Copied" });
              }}
            >
              <Copy className="h-4 w-4" aria-hidden />
            </Button>
          </div>
          <Button className="self-end" onClick={close}>
            Done
          </Button>
        </div>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4">
          <Field id="token-name" label="Name" hint="Name the deployment that uses it, for example eu-west gateways.">
            <Input value={name} onChange={(e) => setName(e.target.value)} autoComplete="off" />
          </Field>
          {issue.isError && <FormError>{errorMessage(issue.error)}</FormError>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name.trim() || issue.isPending}>
              Create
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
