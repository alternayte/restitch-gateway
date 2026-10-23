import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { RefreshCw, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import YAML from "yaml";
import {
  deleteResourceMutation,
  getArtifactSchemaOptions,
  getResourceOptions,
  getResourceQueryKey,
  listDraftSecretNamesOptions,
  listDraftSecretNamesQueryKey,
  listResourcesQueryKey,
  saveResourceMutation,
} from "@/api/@tanstack/react-query.gen";
import { ApiError } from "@/api-client";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DataState } from "@/components/data-state";
import { Button } from "@/components/ui/button";
import { FormError } from "@/components/ui/field";
import { PageHeader } from "@/components/ui/page-header";
import { Tabs } from "@/components/ui/tabs";
import { toast } from "@/components/ui/toast";
import { CodeField } from "@/features/resources/code-field";
import { kindLabel, type Kind } from "@/features/resources/kinds";
import { StepNames } from "@/features/resources/lint";
import { ObjectFields, type Schema } from "@/features/resources/schema-form";
import { SecretNames } from "@/features/resources/secret-picker";
import { StepGraph, type Step } from "@/features/resources/step-graph";
import { useCurrentUser } from "@/lib/auth";
import { errorMessage } from "@/lib/errors";
import { can } from "@/lib/roles";

type Body = Record<string, unknown>;

/** ResourceEditor edits one draft resource as a form or as YAML; both views edit one object. */
export function ResourceEditor({ env, kind, name }: { env: string; kind: Kind; name: string }) {
  const path = { env, kind, name };
  const schema = useQuery(getArtifactSchemaOptions());
  const resource = useQuery({ ...getResourceOptions({ path }), retry: false });
  const isNew = resource.error instanceof ApiError && resource.error.status === 404;

  const query = {
    ...schema,
    data: schema.data && (resource.data || isNew) ? { schema: schema.data, resource: resource.data } : undefined,
    isPending: schema.isPending || (resource.isPending && !isNew),
    isError: schema.isError || (resource.isError && !isNew),
    error: schema.error ?? resource.error,
    refetch: async () => {
      await Promise.all([schema.refetch(), resource.refetch()]);
    },
  };

  return (
    <DataState query={query} skeleton="panel">
      {(d) => {
        const defs = (d.schema.artifact as { $defs: Record<string, Schema> }).$defs;
        const root: Schema =
          kind === "settings" ? (d.schema.settings as Schema) : { $ref: `#/$defs/${d.schema.definitions[kind] ?? ""}` };
        return (
          <EditorForm
            key={`${env}/${kind}/${name}/${d.resource?.revision ?? 0}`}
            env={env}
            kind={kind}
            name={name}
            schema={root}
            defs={defs}
            initial={(d.resource?.body as Body) ?? {}}
            revision={d.resource?.revision ?? 0}
          />
        );
      }}
    </DataState>
  );
}

/** stepNamesOf lists the named steps of a composition body. */
function stepNamesOf(body: Body): string[] {
  const steps = Array.isArray(body.steps) ? body.steps : [];
  return steps.flatMap((s) => (s && typeof s === "object" && typeof (s as Body).name === "string" ? [(s as Body).name as string] : []));
}

function EditorForm({
  env,
  kind,
  name,
  schema,
  defs,
  initial,
  revision,
}: {
  env: string;
  kind: Kind;
  name: string;
  schema: Schema;
  defs: Record<string, Schema>;
  initial: Body;
  revision: number;
}) {
  const me = useCurrentUser();
  const canEdit = can(me.role, "editor");
  const qc = useQueryClient();
  const navigate = useNavigate();
  const path = { env, kind, name };
  const [body, setBody] = useState<Body>(initial);
  const [view, setView] = useState<"form" | "yaml">("form");
  const [yamlText, setYamlText] = useState(() => YAML.stringify(initial));
  const [yamlError, setYamlError] = useState("");
  const [deleting, setDeleting] = useState(false);
  const dirty = JSON.stringify(body) !== JSON.stringify(initial);
  const secrets = useQuery(listDraftSecretNamesOptions({ path: { env } }));
  const secretNames = (secrets.data?.names ?? []).map((n) => n.name);

  useEffect(() => {
    if (view === "yaml") setYamlText(YAML.stringify(body));
    // Only switching to the YAML view re-renders the text from the object.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view]);

  const save = useMutation({
    ...saveResourceMutation(),
    onSuccess: (saved) => {
      qc.setQueryData(getResourceQueryKey({ path }), saved);
      void qc.invalidateQueries({ queryKey: listResourcesQueryKey({ path: { env } }) });
      void qc.invalidateQueries({ queryKey: listDraftSecretNamesQueryKey({ path: { env } }) });
      toast({ title: "Saved", description: `${kindLabel(kind)} ${name}, revision ${saved.revision}` });
    },
  });
  const remove = useMutation({
    ...deleteResourceMutation(),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: listResourcesQueryKey({ path: { env } }) });
      void qc.invalidateQueries({ queryKey: listDraftSecretNamesQueryKey({ path: { env } }) });
      toast({ title: "Deleted", description: `${kindLabel(kind)} ${name}` });
      void navigate({ to: "/environments/$env/draft", params: { env } });
    },
  });
  const stale = save.error instanceof ApiError && save.error.status === 409;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title={`${kindLabel(kind)} ${name}`}
        description={revision === 0 ? `New in the ${env} draft.` : `Revision ${revision} in the ${env} draft.`}
        actions={
          canEdit && (
            <>
              {revision > 0 && (
                <Button variant="secondary" onClick={() => setDeleting(true)}>
                  <Trash2 className="h-4 w-4" aria-hidden />
                  Delete
                </Button>
              )}
              <Button
                disabled={!dirty || save.isPending || yamlError !== ""}
                onClick={() => save.mutate({ path, body: { base_revision: revision, body } })}
              >
                {save.isPending ? "Saving" : "Save"}
              </Button>
            </>
          )
        }
      />
      {stale && (
        <div role="alert" className="flex flex-wrap items-center gap-3 rounded-panel border border-warning/40 bg-warning/10 px-4 py-3 text-sm">
          <span className="flex-1">Someone saved this resource after you opened it. Reload to see their version; your edits are discarded.</span>
          <Button size="sm" variant="secondary" onClick={() => void qc.invalidateQueries({ queryKey: getResourceQueryKey({ path }) })}>
            <RefreshCw className="h-3.5 w-3.5" aria-hidden />
            Reload
          </Button>
        </div>
      )}
      {save.isError && !stale && <FormError>{errorMessage(save.error)}</FormError>}
      <Tabs
        label="Editor view"
        value={view}
        onChange={setView}
        tabs={[
          { value: "form", label: "Form" },
          { value: "yaml", label: "YAML" },
        ]}
      />
      <fieldset disabled={!canEdit} className="min-w-0">
        {view === "form" ? (
          <SecretNames.Provider value={secretNames}>
            <StepNames.Provider value={kind === "composition" ? stepNamesOf(body) : null}>
              {kind === "composition" && (
                <section aria-labelledby="step-graph-heading" className="mb-5 flex flex-col gap-2">
                  <h2 id="step-graph-heading" className="text-sm font-medium">
                    Step graph
                  </h2>
                  <StepGraph
                    steps={Array.isArray(body.steps) ? (body.steps as Step[]) : []}
                    onChange={(steps) => setBody({ ...body, steps })}
                  />
                </section>
              )}
              <ObjectFields schema={schema} defs={defs} value={body} onChange={setBody} path={kind} />
            </StepNames.Provider>
          </SecretNames.Provider>
        ) : (
          <div className="flex flex-col gap-1.5">
            <CodeField
              label="Resource YAML"
              value={yamlText}
              minHeight="20rem"
              onChange={(text) => {
                setYamlText(text);
                try {
                  const parsed: unknown = YAML.parse(text) ?? {};
                  if (typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("The resource must be a mapping.");
                  setBody(parsed as Body);
                  setYamlError("");
                } catch (e) {
                  setYamlError(e instanceof Error ? e.message : "Not valid YAML.");
                }
              }}
            />
            {yamlError ? (
              <p className="text-xs text-destructive">{yamlError}</p>
            ) : (
              <p className="text-xs text-muted-foreground">Comments are not kept: Studio stores the resource, not the text.</p>
            )}
          </div>
        )}
      </fieldset>
      <ConfirmDialog
        open={deleting}
        onClose={() => setDeleting(false)}
        title={`Delete ${kindLabel(kind).toLowerCase()} ${name}?`}
        confirmLabel="Delete"
        destructive
        pending={remove.isPending}
        error={remove.isError ? errorMessage(remove.error) : undefined}
        onConfirm={() => remove.mutate({ path, query: { base_revision: revision } })}
      >
        The resource leaves the draft. Published versions keep it until the next publish.
      </ConfirmDialog>
    </div>
  );
}
