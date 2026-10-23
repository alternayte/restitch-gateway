import { Minus, Plus, X } from "lucide-react";
import { useState } from "react";
import YAML from "yaml";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { CodeField } from "@/features/resources/code-field";
import { useTemplateLint } from "@/features/resources/lint";
import { StringInput } from "@/features/resources/secret-picker";

/** Schema is the subset of JSON Schema that the artifact schema uses. */
export type Schema =
  | boolean
  | {
      $ref?: string;
      type?: string;
      properties?: Record<string, Schema>;
      additionalProperties?: Schema;
      items?: Schema;
      pattern?: string;
      description?: string;
      enum?: string[];
    };

type Defs = Record<string, Schema>;
type Obj = Record<string, unknown>;

/** resolve follows a $ref into the definitions. */
export function resolve(schema: Schema, defs: Defs): Exclude<Schema, boolean> | true {
  if (schema === true || schema === false) return true;
  if (schema.$ref) {
    const name = schema.$ref.replace("#/$defs/", "");
    const target = defs[name];
    return target === undefined ? true : resolve(target, defs);
  }
  return schema;
}

const label = (key: string) => key.replace(/_/g, " ").replace(/^./, (c) => c.toUpperCase());

/**
 * setKey returns obj with key set, or removed when the value is empty. An
 * empty object stays: the user added that section on purpose.
 */
function setKey(obj: Obj, key: string, value: unknown): Obj {
  const next = { ...obj };
  const empty = value === undefined || value === "" || value === false || (Array.isArray(value) && value.length === 0);
  if (empty) delete next[key];
  else next[key] = value;
  return next;
}

/** ObjectFields renders every property of an object schema as a form field. */
export function ObjectFields({
  schema,
  defs,
  value,
  onChange,
  path,
}: {
  schema: Schema;
  defs: Defs;
  value: Obj;
  onChange: (next: Obj) => void;
  path: string;
}) {
  const s = resolve(schema, defs);
  if (s === true || !s.properties) return null;
  return (
    <div className="flex flex-col gap-4">
      {Object.entries(s.properties).map(([key, prop]) => (
        <SchemaField
          key={key}
          name={key}
          path={`${path}.${key}`}
          schema={prop}
          defs={defs}
          value={value[key]}
          onChange={(v) => onChange(setKey(value, key, v))}
        />
      ))}
    </div>
  );
}

/** SchemaField renders one value by the shape of its schema. */
export function SchemaField({
  name,
  path,
  schema,
  defs,
  value,
  onChange,
}: {
  name: string;
  path: string;
  schema: Schema;
  defs: Defs;
  value: unknown;
  onChange: (next: unknown) => void;
}) {
  const s = resolve(schema, defs);
  const id = `f${path.replace(/[^a-zA-Z0-9]/g, "-")}`;

  // A schema of true, or an open object, takes any value: edit it as YAML.
  if (s === true || (s.type === "object" && !s.properties && !s.additionalProperties)) {
    return <FreeField id={id} name={name} value={value} onChange={onChange} json={s !== true} />;
  }
  if (s.type === "object" && s.properties) {
    const present = value !== undefined && value !== null;
    return (
      <fieldset className={present ? "flex flex-col gap-3 rounded-panel border p-3" : "flex flex-col"}>
        <legend className={present ? "flex items-center gap-2 px-1 text-sm font-medium" : "flex items-center gap-2 text-sm font-medium"}>
          {label(name)}
          <Button
            type="button"
            size="sm"
            variant="ghost"
            aria-label={present ? `Remove ${label(name)}` : `Add ${label(name)}`}
            onClick={() => onChange(present ? undefined : {})}
          >
            {present ? <Minus className="h-3.5 w-3.5" aria-hidden /> : <Plus className="h-3.5 w-3.5" aria-hidden />}
          </Button>
        </legend>
        {present && (
          <ObjectFields schema={s} defs={defs} value={value as Obj} onChange={(v) => onChange(v)} path={path} />
        )}
      </fieldset>
    );
  }
  if (s.type === "object" && s.additionalProperties) {
    return <MapField id={id} name={name} value={(value as Record<string, string>) ?? {}} onChange={onChange} />;
  }
  if (s.type === "array") {
    const items = resolve(s.items ?? true, defs);
    const list = Array.isArray(value) ? value : [];
    if (items !== true && items.type === "object") {
      return (
        <fieldset className="flex flex-col gap-3">
          <legend className="text-sm font-medium">{label(name)}</legend>
          {list.map((item, i) => (
            <div key={i} className="flex flex-col gap-3 rounded-panel border p-3">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-muted-foreground">
                  {label(name)} {i + 1}
                </span>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-label={`Remove ${label(name)} ${i + 1}`}
                  onClick={() => onChange(list.filter((_, j) => j !== i))}
                >
                  <X className="h-3.5 w-3.5" aria-hidden />
                </Button>
              </div>
              <ObjectFields
                schema={items}
                defs={defs}
                value={(item as Obj) ?? {}}
                onChange={(v) => onChange(list.map((x, j) => (j === i ? v : x)))}
                path={`${path}.${i}`}
              />
            </div>
          ))}
          <Button type="button" variant="secondary" size="sm" className="self-start" onClick={() => onChange([...list, {}])}>
            <Plus className="h-3.5 w-3.5" aria-hidden />
            Add {label(name).toLowerCase()}
          </Button>
        </fieldset>
      );
    }
    const numeric = items !== true && items.type === "integer";
    return (
      <ScalarField
        id={id}
        name={name}
        hint="Separate values with commas."
        value={list.join(", ")}
        onChange={(text) => {
          const parts = text.split(",").map((p) => p.trim()).filter(Boolean);
          onChange(numeric ? parts.map(Number).filter((n) => !Number.isNaN(n)) : parts);
        }}
      />
    );
  }
  if (s.type === "boolean") {
    return (
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={value === true} onChange={(e) => onChange(e.target.checked)} />
        {label(name)}
      </label>
    );
  }
  if (s.type === "integer" || s.type === "number") {
    return (
      <ScalarField
        id={id}
        name={name}
        type="number"
        value={value === undefined ? "" : String(value)}
        onChange={(text) => onChange(text === "" ? undefined : Number(text))}
      />
    );
  }
  return (
    <ScalarField
      id={id}
      name={name}
      hint={s.description}
      pattern={s.pattern}
      value={typeof value === "string" ? value : ""}
      onChange={(text) => onChange(text)}
    />
  );
}

function ScalarField({
  id,
  name,
  value,
  onChange,
  hint,
  pattern,
  type = "text",
}: {
  id: string;
  name: string;
  value: string;
  onChange: (text: string) => void;
  hint?: string;
  pattern?: string;
  type?: string;
}) {
  const invalid = pattern !== undefined && value !== "" && !new RegExp(pattern).test(value);
  const lintError = useTemplateLint(type === "text" ? value : "");
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-sm font-medium">
        {label(name)}
      </label>
      {/* A field with a pattern, such as a duration, cannot hold a ${NAME}: it gets no secret picker. */}
      {type === "text" && pattern === undefined ? (
        <StringInput id={id} fieldLabel={label(name)} value={value} aria-invalid={invalid || lintError !== "" || undefined} onChange={onChange} />
      ) : (
        <Input
          id={id}
          type={type}
          value={value}
          aria-invalid={invalid || lintError !== "" || undefined}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
      {invalid && <p className="text-xs text-destructive">{hint ?? "This value has the wrong format."}</p>}
      {!invalid && lintError && (
        <p role="alert" className="font-mono text-xs break-words text-destructive">
          {lintError}
        </p>
      )}
      {!invalid && !lintError && hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

function MapField({
  id,
  name,
  value,
  onChange,
}: {
  id: string;
  name: string;
  value: Record<string, string>;
  onChange: (next: Record<string, string> | undefined) => void;
}) {
  const entries = Object.entries(value);
  const [draftKey, setDraftKey] = useState("");
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="text-sm font-medium">{label(name)}</legend>
      {entries.map(([k, v]) => (
        <div key={k} className="flex items-center gap-2">
          <span className="w-40 truncate font-mono text-xs">{k}</span>
          <StringInput aria-label={`${label(name)} ${k}`} fieldLabel={`${label(name)} ${k}`} value={v} onChange={(text) => onChange({ ...value, [k]: text })} />
          <Button
            type="button"
            size="sm"
            variant="ghost"
            aria-label={`Remove ${k}`}
            onClick={() => {
              const next = { ...value };
              delete next[k];
              onChange(Object.keys(next).length ? next : undefined);
            }}
          >
            <X className="h-3.5 w-3.5" aria-hidden />
          </Button>
        </div>
      ))}
      <div className="flex items-center gap-2">
        <Input id={id} placeholder="Name" value={draftKey} onChange={(e) => setDraftKey(e.target.value)} className="w-40" />
        <Button
          type="button"
          size="sm"
          variant="secondary"
          disabled={!draftKey.trim() || draftKey.trim() in value}
          onClick={() => {
            onChange({ ...value, [draftKey.trim()]: "" });
            setDraftKey("");
          }}
        >
          Add
        </Button>
      </div>
    </fieldset>
  );
}

/** FreeField edits a value of any shape as YAML, or JSON for a JSON Schema. */
function FreeField({
  id,
  name,
  value,
  onChange,
  json,
}: {
  id: string;
  name: string;
  value: unknown;
  onChange: (next: unknown) => void;
  json: boolean;
}) {
  const render = (v: unknown) => (v === undefined ? "" : json ? JSON.stringify(v, null, 2) : YAML.stringify(v).trimEnd());
  const [text, setText] = useState(() => render(value));
  const [error, setError] = useState("");
  return (
    <div className="flex flex-col gap-1.5" id={id}>
      <span className="text-sm font-medium">{label(name)}</span>
      <CodeField
        label={label(name)}
        language={json ? "json" : "yaml"}
        value={text}
        onChange={(next) => {
          setText(next);
          if (next.trim() === "") {
            setError("");
            onChange(undefined);
            return;
          }
          try {
            onChange(json ? JSON.parse(next) : YAML.parse(next));
            setError("");
          } catch (e) {
            setError(e instanceof Error ? e.message : "Not valid.");
          }
        }}
      />
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  );
}
