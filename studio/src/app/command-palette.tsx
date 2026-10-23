import { useQueries, useQuery } from "@tanstack/react-query";
import { useNavigate, useRouterState } from "@tanstack/react-router";
import {
  CornerDownLeft,
  FileCode2,
  FilePlus,
  Keyboard,
  Layers,
  LogOut,
  Monitor,
  Moon,
  Plus,
  Search,
  Sun,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { listEnvironmentsOptions, listResourcesOptions } from "@/api/@tanstack/react-query.gen";
import { currentEnv, envPages, navItems, settingsItems } from "@/app/nav";
import { CreateEnvironmentDialog } from "@/features/environments/EnvironmentsPage";
import { NewResourceDialog } from "@/features/resources/DraftPage";
import { kindLabel, kinds } from "@/features/resources/kinds";
import { useCurrentUser, useSignOut } from "@/lib/auth";
import { matchScore, type Command } from "@/lib/commands";
import { can } from "@/lib/roles";
import { useTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";

type Item = Command & { rank?: number };

const maxPerGroup = 8;
const resourceEnvLimit = 25;

/** usePaletteItems builds the palette entries from the API data. */
function usePaletteItems(
  open: boolean,
  onShortcuts: () => void,
  onNewEnvironment: () => void,
  onNewResource: (env: string) => void,
) {
  const me = useCurrentUser();
  const navigate = useNavigate();
  const { setTheme } = useTheme();
  const { signOut } = useSignOut();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const here = currentEnv(pathname);

  const environments = useQuery({ ...listEnvironmentsOptions(), enabled: open, staleTime: 30_000 });
  const envNames = (environments.data?.environments ?? []).slice(0, resourceEnvLimit).map((e) => e.name);
  const resources = useQueries({
    queries: envNames.map((env) => ({
      ...listResourcesOptions({ path: { env } }),
      enabled: open,
      staleTime: 30_000,
    })),
  });
  const resourcesKey = resources.map((r) => r.dataUpdatedAt).join(",");

  return useMemo(() => {
    const out: Item[] = [];

    if (can(me.role, "owner")) {
      out.push({
        id: "new-environment",
        group: "Actions",
        label: "New environment",
        icon: Plus,
        keywords: "create add environment",
        run: onNewEnvironment,
      });
    }
    if (can(me.role, "editor")) {
      for (const env of envNames) {
        out.push({
          id: `new-resource-${env}`,
          group: "Actions",
          label: `New resource in ${env}`,
          icon: FilePlus,
          keywords: `create add resource ${kinds.map((k) => k.label).join(" ")}`,
          when: env === here ? undefined : "search",
          run: () => onNewResource(env),
        });
      }
    }
    out.push(
      { id: "shortcuts", group: "Actions", label: "Show keyboard shortcuts", icon: Keyboard, hint: "?", run: onShortcuts },
      // Theme and sign-out show only in search results, so the empty palette leads with pages.
      { id: "theme-light", group: "Actions", label: "Switch to light theme", icon: Sun, keywords: "appearance", when: "search", run: () => setTheme("light") },
      { id: "theme-dark", group: "Actions", label: "Switch to dark theme", icon: Moon, keywords: "appearance", when: "search", run: () => setTheme("dark") },
      { id: "theme-system", group: "Actions", label: "Use the system theme", icon: Monitor, keywords: "appearance", when: "search", run: () => setTheme("system") },
      { id: "sign-out", group: "Actions", label: "Sign out", icon: LogOut, keywords: "log out logout", when: "search", run: () => void signOut() },
    );

    for (const item of [...navItems, ...settingsItems]) {
      if (!can(me.role, item.min)) continue;
      out.push({
        id: `nav-${item.to}`,
        group: "Go to",
        label: item.label,
        icon: item.icon,
        hint: item.chord ? `G ${item.chord.toUpperCase()}` : undefined,
        keywords: "page open go",
        run: () => void navigate({ to: item.to }),
      });
    }
    const pages = envPages.filter((p) => can(me.role, p.min));
    if (here) {
      for (const p of pages) {
        out.push({
          id: `here-${p.to}`,
          group: "Go to",
          label: p.label,
          icon: p.icon,
          hint: `G ${p.chord.toUpperCase()}`,
          when: "idle",
          run: () => void navigate({ to: p.to, params: { env: here } }),
        });
      }
    }

    for (const env of envNames) {
      out.push({
        id: `env-${env}`,
        group: "Environments",
        label: env,
        icon: Layers,
        hint: env === here ? "Current" : undefined,
        keywords: "environment open overview",
        run: () => void navigate({ to: "/environments/$env", params: { env } }),
      });
      for (const p of pages) {
        if (p.to === "/environments/$env") continue;
        out.push({
          id: `env-${env}-${p.to}`,
          group: "Environments",
          label: `${env} / ${p.label}`,
          icon: p.icon,
          keywords: p.keywords,
          when: "search",
          run: () => void navigate({ to: p.to, params: { env } }),
        });
      }
    }

    resources.forEach((q, i) => {
      const env = envNames[i];
      if (!env) return;
      for (const r of q.data?.resources ?? []) {
        const singleton = kinds.find((k) => k.kind === r.kind)?.singleton ?? false;
        out.push({
          id: `res-${env}-${r.kind}-${r.name}`,
          group: "Resources",
          label: singleton ? kindLabel(r.kind) : r.name,
          icon: FileCode2,
          hint: singleton ? env : `${env} · ${kindLabel(r.kind)}`,
          keywords: `${r.name} ${r.kind} resource`,
          when: "search",
          run: () =>
            void navigate({
              to: "/environments/$env/resources/$kind/$name",
              params: { env, kind: r.kind, name: r.name },
            }),
        });
      }
    });
    return out;
    // resources changes identity on every render; its data updates are what matter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [environments.data, resourcesKey, here, me.role, navigate, setTheme, signOut, onShortcuts, onNewEnvironment, onNewResource]);
}

// Groups in the order they show.
const groupOrder = ["Actions", "Go to", "Environments", "Resources"];

function rankItems(items: Item[], query: string): { group: string; items: Item[] }[] {
  const searching = query.trim() !== "";
  const scored = items
    .filter((it) => it.when !== (searching ? "idle" : "search"))
    .map((it) => ({ ...it, rank: matchScore(query, `${it.label} ${it.keywords ?? ""} ${it.hint ?? ""}`) }))
    .filter((it) => it.rank > 0);
  const groups = new Map<string, Item[]>();
  for (const it of scored) {
    const list = groups.get(it.group) ?? [];
    list.push(it);
    groups.set(it.group, list);
  }
  const out = [...groups.entries()].map(([group, list]) => ({
    group,
    items: (searching ? list.sort((a, b) => (b.rank ?? 0) - (a.rank ?? 0)) : list).slice(0, maxPerGroup),
  }));
  // A search shows the group with the best match first, so Enter picks it.
  const best = (g: { items: Item[] }) => g.items[0]?.rank ?? 0;
  return out.sort((a, b) =>
    searching && best(a) !== best(b) ? best(b) - best(a) : groupOrder.indexOf(a.group) - groupOrder.indexOf(b.group),
  );
}

/** CommandPalette is the Cmd+K palette: search pages, environments and resources, and run actions. */
export function CommandPalette({
  open,
  onClose,
  onShortcuts,
}: {
  open: boolean;
  onClose: () => void;
  onShortcuts: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  // The content stays mounted until the close transition ends. Closed, the palette has no controls in the page.
  const [rendered, setRendered] = useState(open);
  if (open && !rendered) setRendered(true);
  const [creatingEnv, setCreatingEnv] = useState(false);
  const [resourceEnv, setResourceEnv] = useState<string>();

  const onNewEnvironment = useMemo(() => () => setCreatingEnv(true), []);
  const onNewResource = useMemo(() => (env: string) => setResourceEnv(env), []);
  const items = usePaletteItems(open, onShortcuts, onNewEnvironment, onNewResource);
  const groups = useMemo(() => rankItems(items, query), [items, query]);
  const flat = groups.flatMap((g) => g.items);
  const current = Math.min(active, Math.max(flat.length - 1, 0));

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      setQuery("");
      setActive(0);
      d.showModal();
      inputRef.current?.focus();
    }
    if (!open && d.open) d.close();
    if (open) return;
    const t = window.setTimeout(() => setRendered(false), 220);
    return () => window.clearTimeout(t);
  }, [open]);

  useEffect(() => {
    listRef.current?.querySelector('[aria-selected="true"]')?.scrollIntoView?.({ block: "nearest" });
  }, [current, query]);

  const choose = (it: Item | undefined) => {
    if (!it) return;
    onClose();
    it.run();
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    const down = e.key === "ArrowDown" || (e.ctrlKey && e.key === "n");
    const up = e.key === "ArrowUp" || (e.ctrlKey && e.key === "p");
    if (down || up) {
      e.preventDefault();
      if (flat.length) setActive((current + (down ? 1 : -1) + flat.length) % flat.length);
    } else if (e.key === "Enter") {
      e.preventDefault();
      choose(flat[current]);
    }
  };

  let index = -1;
  return (
    <>
      <dialog
        ref={ref}
        onClose={onClose}
        // Escape closes through state. Chromium does not always deliver the close event after a
        // close request, which left the palette closed while its state said open.
        onCancel={(e) => {
          e.preventDefault();
          onClose();
        }}
        onClick={(e) => {
          if (e.target === ref.current) onClose();
        }}
        aria-label="Command palette"
        className="sheet mx-auto mt-[12vh] mb-auto w-[calc(100%-2rem)] max-w-xl overflow-hidden rounded-panel border bg-panel/90 p-0 text-foreground shadow-float backdrop-blur-2xl backdrop-saturate-150"
      >
        {rendered && (
          <>
            <div className="flex items-center gap-2.5 border-b px-4">
              <Search className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />
              <input
                ref={inputRef}
                role="combobox"
                aria-expanded="true"
                aria-controls="palette-list"
                aria-activedescendant={flat[current] ? `palette-${flat[current].id}` : undefined}
                aria-label="Search or run a command"
                placeholder="Search pages, environments and resources"
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value);
                  setActive(0);
                }}
                onKeyDown={onKeyDown}
                className="h-12 min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground"
                autoComplete="off"
                spellCheck={false}
              />
              <kbd className="rounded-inner border px-1.5 font-sans text-xs text-muted-foreground">esc</kbd>
            </div>
            <div
              ref={listRef}
              id="palette-list"
              role="listbox"
              aria-label="Results"
              className="max-h-[min(60vh,26rem)] overflow-y-auto p-1.5"
            >
              {flat.length === 0 && <p className="px-3 py-8 text-center text-sm text-muted-foreground">No results.</p>}
              {groups.map((g) => (
                <div key={g.group} role="group" aria-label={g.group} className="pb-1">
                  <div className="px-2.5 pt-2 pb-1 text-xs font-medium text-muted-foreground">{g.group}</div>
                  {g.items.map((it) => {
                    index++;
                    const selected = index === current;
                    const i = index;
                    const Icon: LucideIcon | undefined = it.icon;
                    return (
                      <div
                        key={it.id}
                        id={`palette-${it.id}`}
                        role="option"
                        aria-selected={selected}
                        onMouseMove={() => active !== i && setActive(i)}
                        onClick={() => choose(it)}
                        className={cn(
                          "flex h-9 cursor-default items-center gap-2.5 rounded-control px-2.5 text-sm",
                          selected ? "bg-accent-fill text-accent-foreground" : "text-foreground",
                        )}
                      >
                        {Icon && (
                          <Icon className={cn("h-4 w-4 shrink-0", selected ? "" : "text-muted-foreground")} aria-hidden />
                        )}
                        <span className="min-w-0 flex-1 truncate">{it.label}</span>
                        {it.hint && (
                          <span
                            className={cn(
                              "max-w-[40%] shrink-0 truncate text-xs",
                              selected ? "text-accent-foreground/80" : "text-muted-foreground",
                            )}
                          >
                            {it.hint}
                          </span>
                        )}
                        {selected && <CornerDownLeft className="h-3.5 w-3.5 shrink-0 opacity-80" aria-hidden />}
                      </div>
                    );
                  })}
                </div>
              ))}
            </div>
            <div className="flex items-center gap-4 border-t px-4 py-2 text-xs text-muted-foreground">
              <span>
                <kbd className="font-sans">↑↓</kbd> move
              </span>
              <span>
                <kbd className="font-sans">↵</kbd> open
              </span>
              <span className="ml-auto">
                <kbd className="font-sans">?</kbd> shortcuts
              </span>
            </div>
          </>
        )}
      </dialog>
      <CreateEnvironmentDialog open={creatingEnv} onClose={() => setCreatingEnv(false)} />
      {resourceEnv && (
        <NewResourceDialog env={resourceEnv} open onClose={() => setResourceEnv(undefined)} />
      )}
    </>
  );
}
