import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { FileCode2, Plus } from "lucide-react";
import { useState, type FormEvent } from "react";
import { listResourcesOptions } from "@/api/@tanstack/react-query.gen";
import { DataState } from "@/components/data-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { PageHeader } from "@/components/ui/page-header";
import { TBody, THead, Table, Td, Th, Tr } from "@/components/ui/table";
import { kindLabel, kinds, type Kind } from "@/features/resources/kinds";
import { useCurrentUser } from "@/lib/auth";
import { can } from "@/lib/roles";
import { formatTime } from "@/lib/utils";

/** DraftPage lists the resources in an environment's draft. */
export function DraftPage({ env }: { env: string }) {
  const me = useCurrentUser();
  const query = useQuery(listResourcesOptions({ path: { env } }));
  const [adding, setAdding] = useState(false);
  const add = can(me.role, "editor") ? (
    <Button onClick={() => setAdding(true)}>
      <Plus className="h-4 w-4" aria-hidden />
      New resource
    </Button>
  ) : undefined;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader title={env} description="The draft: edit resources here, then publish a version." actions={add} />
      <DataState
        query={query}
        empty={(d) => (d.resources ?? []).length === 0}
        emptyIcon={FileCode2}
        emptyText="The draft is empty. Add an upstream, then a composition that calls it."
        emptyAction={add}
      >
        {(d) => (
          <Table>
            <THead>
              <Tr>
                <Th>Kind</Th>
                <Th>Name</Th>
                <Th>Revision</Th>
                <Th>Updated</Th>
                <Th>By</Th>
              </Tr>
            </THead>
            <TBody>
              {(d.resources ?? []).map((r) => (
                <Tr key={`${r.kind}/${r.name}`}>
                  <Td>
                    <Badge>{kindLabel(r.kind)}</Badge>
                  </Td>
                  <Td className="font-medium">
                    <Link
                      to="/environments/$env/resources/$kind/$name"
                      params={{ env, kind: r.kind, name: r.name }}
                      className="hover:underline"
                    >
                      {r.name}
                    </Link>
                  </Td>
                  <Td>{r.revision}</Td>
                  <Td>{formatTime(r.updated_at)}</Td>
                  <Td>{r.updated_by}</Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </DataState>
      <NewResourceDialog env={env} open={adding} onClose={() => setAdding(false)} />
    </div>
  );
}

function NewResourceDialog({ env, open, onClose }: { env: string; open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  const [kind, setKind] = useState<Kind>("upstream");
  const [name, setName] = useState("");
  const singleton = kinds.find((k) => k.kind === kind)?.singleton ?? false;
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const target = singleton ? "default" : name.trim();
    if (!target) return;
    onClose();
    void navigate({ to: "/environments/$env/resources/$kind/$name", params: { env, kind, name: target } });
  };
  return (
    <Dialog open={open} onClose={onClose} title="New resource">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field id="resource-kind" label="Kind">
          <select
            className="h-8 rounded-control border bg-panel px-2 text-sm"
            value={kind}
            onChange={(e) => setKind(e.target.value as Kind)}
          >
            {kinds.map((k) => (
              <option key={k.kind} value={k.kind}>
                {k.label}
              </option>
            ))}
          </select>
        </Field>
        {!singleton && (
          <Field id="resource-name" label="Name" hint="Letters, digits, dots, dashes and underscores.">
            <Input value={name} onChange={(e) => setName(e.target.value)} autoComplete="off" />
          </Field>
        )}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!singleton && !name.trim()}>
            Continue
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
