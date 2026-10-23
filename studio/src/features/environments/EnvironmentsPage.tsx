import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Layers, Plus } from "lucide-react";
import { useState, type FormEvent } from "react";
import {
  createEnvironmentMutation,
  listEnvironmentsOptions,
  listEnvironmentsQueryKey,
} from "@/api/@tanstack/react-query.gen";
import { DataState } from "@/components/data-state";
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

const namePattern = /^[a-z][a-z0-9-]{0,62}$/;

export function EnvironmentsPage() {
  const me = useCurrentUser();
  const query = useQuery(listEnvironmentsOptions());
  const [creating, setCreating] = useState(false);
  const isOwner = can(me.role, "owner");

  const create = isOwner ? (
    <Button onClick={() => setCreating(true)}>
      <Plus className="h-4 w-4" aria-hidden />
      New environment
    </Button>
  ) : undefined;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Environments"
        description="Each environment has one draft and its published versions. Gateways pull the current version."
        actions={create}
      />
      <DataState
        query={query}
        empty={(d) => (d.environments ?? []).length === 0}
        emptyIcon={Layers}
        emptyText={isOwner ? "No environment yet. Create one to start a draft." : "No environment yet. An owner creates them."}
        emptyAction={create}
      >
        {(data) => (
          <Table>
            <THead>
              <Tr>
                <Th>Name</Th>
                <Th>Created</Th>
                <Th>Created by</Th>
              </Tr>
            </THead>
            <TBody>
              {(data.environments ?? []).map((env) => (
                <Tr key={env.name}>
                  <Td className="font-medium">
                    <Link to="/environments/$env" params={{ env: env.name }} className="hover:underline">
                      {env.name}
                    </Link>
                  </Td>
                  <Td>{formatTime(env.created_at)}</Td>
                  <Td>{env.created_by}</Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </DataState>
      <CreateEnvironmentDialog open={creating} onClose={() => setCreating(false)} />
    </div>
  );
}

export function CreateEnvironmentDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [invalid, setInvalid] = useState(false);
  const mutation = useMutation({
    ...createEnvironmentMutation(),
    onSuccess: (env) => {
      void qc.invalidateQueries({ queryKey: listEnvironmentsQueryKey() });
      toast({ title: "Environment created", description: env.name });
      setName("");
      onClose();
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!namePattern.test(name)) {
      setInvalid(true);
      return;
    }
    setInvalid(false);
    mutation.mutate({ body: { name } });
  };

  return (
    <Dialog open={open} onClose={onClose} title="New environment">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field
          id="env-name"
          label="Name"
          hint="Lowercase letters, digits and dashes, starting with a letter. For example staging or prod."
          error={invalid ? "Use lowercase letters, digits and dashes, starting with a letter." : undefined}
        >
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus autoComplete="off" />
        </Field>
        {mutation.isError && <FormError>{errorMessage(mutation.error)}</FormError>}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? "Creating" : "Create"}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
