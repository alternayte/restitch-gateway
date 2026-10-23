import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Users } from "lucide-react";
import { useState, type FormEvent } from "react";
import {
  adminCreateUserMutation,
  adminDisableUserMutation,
  adminEnableUserMutation,
  adminListUsersOptions,
  adminListUsersQueryKey,
  adminResetUserPasswordMutation,
  adminSetUserRoleMutation,
} from "@/api/@tanstack/react-query.gen";
import type { AdminUser } from "@/api/types.gen";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DataState } from "@/components/data-state";
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
import { roleLabels, roles, type Role } from "@/lib/roles";

const selectClass = "h-8 rounded-control border bg-panel px-2 text-sm";

/** UsersPage lets owners invite people, change roles, and disable access. */
export function UsersPage() {
  const me = useCurrentUser();
  const qc = useQueryClient();
  const query = useQuery(adminListUsersOptions());
  const [inviting, setInviting] = useState(false);
  const [disabling, setDisabling] = useState<AdminUser | null>(null);
  const [password, setPassword] = useState<{ email: string; value: string } | null>(null);
  const refresh = () => void qc.invalidateQueries({ queryKey: adminListUsersQueryKey() });
  const onError = (e: unknown) => toast({ title: "Not changed", description: errorMessage(e), tone: "error" });

  const setRole = useMutation({ ...adminSetUserRoleMutation(), onSuccess: refresh, onError });
  const enable = useMutation({ ...adminEnableUserMutation(), onSuccess: refresh, onError });
  const disable = useMutation({
    ...adminDisableUserMutation(),
    onSuccess: () => {
      refresh();
      setDisabling(null);
    },
  });
  const reset = useMutation({
    ...adminResetUserPasswordMutation(),
    onSuccess: (out, vars) => {
      const user = (query.data?.users ?? []).find((u) => u.id === vars.path.id);
      if (out.temporaryPassword) setPassword({ email: user?.email ?? "", value: out.temporaryPassword });
    },
    onError,
  });

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Users"
        description="Viewers read, editors change and publish drafts, owners also manage environments, tokens and users."
        actions={
          <Button onClick={() => setInviting(true)}>
            <Plus className="h-4 w-4" aria-hidden />
            Add user
          </Button>
        }
      />
      <DataState query={query} empty={(d) => d.users.length === 0} emptyIcon={Users} emptyText="No users.">
        {(d) => (
          <Table>
            <THead>
              <Tr>
                <Th>Email</Th>
                <Th>Role</Th>
                <Th>Status</Th>
                <Th className="text-right">Actions</Th>
              </Tr>
            </THead>
            <TBody>
              {d.users.map((u) => {
                const self = u.id === me.id;
                return (
                  <Tr key={u.id}>
                    <Td className="font-medium">
                      {u.email} {self && <Badge>you</Badge>}
                    </Td>
                    <Td>
                      <select
                        aria-label={`Role of ${u.email}`}
                        className={selectClass}
                        value={u.role || "viewer"}
                        disabled={self || setRole.isPending}
                        onChange={(e) => setRole.mutate({ path: { id: u.id }, body: { role: e.target.value } })}
                      >
                        {roles.map((r) => (
                          <option key={r} value={r}>
                            {roleLabels[r]}
                          </option>
                        ))}
                      </select>
                    </Td>
                    <Td>
                      {u.disabledAt ? <Badge>disabled</Badge> : u.mustChangePassword ? <Badge>temporary password</Badge> : "Active"}
                    </Td>
                    <Td className="text-right">
                      {!self && (
                        <div className="flex justify-end gap-1">
                          <Button size="sm" variant="ghost" onClick={() => reset.mutate({ path: { id: u.id }, body: {} })}>
                            Reset password
                          </Button>
                          {u.disabledAt ? (
                            <Button size="sm" variant="ghost" onClick={() => enable.mutate({ path: { id: u.id } })}>
                              Enable
                            </Button>
                          ) : (
                            <Button size="sm" variant="ghost" onClick={() => setDisabling(u)}>
                              Disable
                            </Button>
                          )}
                        </div>
                      )}
                    </Td>
                  </Tr>
                );
              })}
            </TBody>
          </Table>
        )}
      </DataState>
      <InviteDialog
        open={inviting}
        onClose={() => setInviting(false)}
        onCreated={(email, value) => {
          refresh();
          if (value) setPassword({ email, value });
        }}
      />
      <ConfirmDialog
        open={disabling !== null}
        onClose={() => setDisabling(null)}
        title={`Disable ${disabling?.email ?? ""}?`}
        confirmLabel="Disable"
        destructive
        pending={disable.isPending}
        error={disable.isError ? errorMessage(disable.error) : undefined}
        onConfirm={() => disabling && disable.mutate({ path: { id: disabling.id } })}
      >
        Every session and API key of this user stops working at once. You can enable the user again.
      </ConfirmDialog>
      <Dialog open={password !== null} onClose={() => setPassword(null)} title="Temporary password">
        <div className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">
            Give it to {password?.email} through a private channel. They set a new password at their first sign-in. It is
            shown only now.
          </p>
          <Input readOnly value={password?.value ?? ""} aria-label="Temporary password" className="font-mono" onFocus={(e) => e.target.select()} />
          <Button className="self-end" onClick={() => setPassword(null)}>
            Done
          </Button>
        </div>
      </Dialog>
    </div>
  );
}

function InviteDialog({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: (email: string, temporaryPassword?: string) => void;
}) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("viewer");
  const create = useMutation({
    ...adminCreateUserMutation(),
    onSuccess: (out) => {
      onCreated(out.user.email, out.temporaryPassword);
      setEmail("");
      setRole("viewer");
      onClose();
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    create.mutate({ body: { email: email.trim(), role, temporaryPassword: true } });
  };
  return (
    <Dialog open={open} onClose={onClose} title="Add user">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field id="invite-email" label="Email">
          <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" />
        </Field>
        <Field id="invite-role" label="Role">
          <select className={selectClass} value={role} onChange={(e) => setRole(e.target.value as Role)}>
            {roles.map((r) => (
              <option key={r} value={r}>
                {roleLabels[r]}
              </option>
            ))}
          </select>
        </Field>
        <p className="text-xs text-muted-foreground">Studio generates a temporary password. People who sign in through OIDC need no invite.</p>
        {create.isError && <FormError>{errorMessage(create.error)}</FormError>}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!email.trim() || create.isPending}>
            Add
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
