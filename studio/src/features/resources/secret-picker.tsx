import { Link } from "@tanstack/react-router";
import { KeyRound } from "lucide-react";
import { createContext, useContext, useEffect, useId, useRef, useState, type InputHTMLAttributes, type KeyboardEvent } from "react";
import type { RequiredName } from "@/api/types.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { TBody, THead, Table, Td, Th, Tr } from "@/components/ui/table";
import { kindLabel, type Kind } from "@/features/resources/kinds";

/** VAR_NAME is the syntax of a name inside ${...}; it matches gwconfig.VarName. */
export const VAR_NAME = /^[A-Za-z_][A-Za-z0-9_]*$/;

/** SecretNames gives string fields the ${NAME} names the environment already uses. Null hides the picker. */
export const SecretNames = createContext<string[] | null>(null);

/** insertReference puts ${name} over the selection [start, end) and returns the text and the caret after it. */
export function insertReference(text: string, start: number, end: number, name: string): { text: string; caret: number } {
  const ref = `\${${name}}`;
  return { text: text.slice(0, start) + ref + text.slice(end), caret: start + ref.length };
}

type StringInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange"> & {
  value: string;
  onChange: (text: string) => void;
  /** fieldLabel names the field in the picker button's label. */
  fieldLabel: string;
};

/** StringInput is a text input with a picker that inserts a ${NAME} reference at the cursor. */
export function StringInput({ value, onChange, fieldLabel, ...props }: StringInputProps) {
  const names = useContext(SecretNames);
  const ref = useRef<HTMLInputElement>(null);
  const caret = useRef<number | null>(null);

  useEffect(() => {
    const input = ref.current;
    if (caret.current === null || !input) return;
    input.focus();
    input.setSelectionRange(caret.current, caret.current);
    caret.current = null;
  }, [value]);

  const input = <Input ref={ref} value={value} onChange={(e) => onChange(e.target.value)} {...props} />;
  if (names === null) return input;
  return (
    <div className="flex min-w-0 flex-1 items-center gap-1">
      {input}
      <SecretPicker
        names={names}
        label={fieldLabel}
        onPick={(name) => {
          const el = ref.current;
          const start = el?.selectionStart ?? value.length;
          const end = el?.selectionEnd ?? value.length;
          const next = insertReference(value, start, end, name);
          caret.current = next.caret;
          onChange(next.text);
        }}
        onDismiss={() => ref.current?.focus()}
      />
    </div>
  );
}

function SecretPicker({
  names,
  label,
  onPick,
  onDismiss,
}: {
  names: string[];
  label: string;
  onPick: (name: string) => void;
  onDismiss: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const root = useRef<HTMLDivElement>(null);
  const panelId = useId();

  useEffect(() => {
    if (!open) return;
    const away = (e: PointerEvent) => {
      if (root.current && !root.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", away);
    return () => document.removeEventListener("pointerdown", away);
  }, [open]);

  const q = query.trim();
  const shown = names.filter((n) => n.toLowerCase().includes(q.toLowerCase()));
  const isNew = q !== "" && !names.includes(q);
  const valid = VAR_NAME.test(q);
  const close = () => {
    setOpen(false);
    setQuery("");
  };
  const pick = (name: string) => {
    close();
    onPick(name);
  };
  const onKey = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      close();
      onDismiss();
    }
    if (e.key === "Enter") {
      e.preventDefault();
      if (q !== "" && names.includes(q)) pick(q);
      else if (isNew && valid) pick(q);
      else if (shown[0] !== undefined) pick(shown[0]);
    }
  };

  return (
    <div ref={root} className="relative">
      <Button
        variant="ghost"
        size="icon"
        aria-label={`Insert a secret name into ${label}`}
        title="Insert a ${NAME} secret reference"
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        // Keep the input's selection: the button must not take focus on press.
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => (open ? close() : setOpen(true))}
      >
        <KeyRound className="h-3.5 w-3.5" aria-hidden />
      </Button>
      {open && (
        <div
          id={panelId}
          role="dialog"
          aria-label="Secret names"
          onKeyDown={onKey}
          className="absolute right-0 z-20 mt-1 flex w-72 max-w-[calc(100vw-2rem)] animate-enter flex-col gap-2 rounded-panel border bg-panel p-2 shadow-float"
        >
          <Input
            autoFocus
            aria-label="Secret name"
            placeholder="Find or type a new name"
            value={query}
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
            onChange={(e) => setQuery(e.target.value)}
          />
          {shown.length > 0 && (
            <ul className="flex max-h-48 flex-col overflow-auto">
              {shown.map((n) => (
                <li key={n}>
                  <button
                    type="button"
                    className="w-full rounded-control px-2 py-1 text-left font-mono text-xs hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                    onClick={() => pick(n)}
                  >
                    {"${"}
                    {n}
                    {"}"}
                  </button>
                </li>
              ))}
            </ul>
          )}
          {isNew &&
            (valid ? (
              <button
                type="button"
                className="rounded-control px-2 py-1 text-left text-xs hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                onClick={() => pick(q)}
              >
                Insert new name <span className="font-mono">{`\${${q}}`}</span>
              </button>
            ) : (
              <p role="alert" className="px-2 text-xs text-destructive">
                Use letters, digits and _, and do not start with a digit.
              </p>
            ))}
          {names.length === 0 && q === "" && (
            <p className="px-2 text-xs text-muted-foreground">No names yet. Type one, for example API_TOKEN.</p>
          )}
          <p className="border-t px-2 pt-2 text-xs text-muted-foreground">
            The gateway reads the value from its environment. Studio stores only the name.
          </p>
        </div>
      )}
    </div>
  );
}

/** RequiredNames lists the ${NAME} references that a gateway needs set to run this config. */
export function RequiredNames({ env, names, linkResources = false }: { env: string; names: RequiredName[]; linkResources?: boolean }) {
  if (names.length === 0) {
    return <p className="text-sm text-muted-foreground">No resource references a ${"{NAME}"}. The gateway needs no secrets.</p>;
  }
  return (
    <Table>
      <THead>
        <Tr>
          <Th>Name</Th>
          <Th>Used by</Th>
        </Tr>
      </THead>
      <TBody>
        {names.map((n) => (
          <Tr key={n.name}>
            <Td className="font-mono text-xs whitespace-nowrap">
              {n.name} {n.optional && <Badge>has default</Badge>}
            </Td>
            <Td>
              <span className="flex flex-wrap gap-x-3 gap-y-1">
                {(n.used_by ?? []).map((u) => {
                  const [kind, name] = u.split("/") as [Kind, string];
                  const text = `${kindLabel(kind)} ${name}`;
                  return linkResources ? (
                    <Link
                      key={u}
                      to="/environments/$env/resources/$kind/$name"
                      params={{ env, kind, name }}
                      className="hover:underline"
                    >
                      {text}
                    </Link>
                  ) : (
                    <span key={u}>{text}</span>
                  );
                })}
              </span>
            </Td>
          </Tr>
        ))}
      </TBody>
    </Table>
  );
}
