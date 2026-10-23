import type { LucideIcon } from "lucide-react";

/** Command is one entry of the command palette. */
export type Command = {
  id: string;
  label: string;
  /** group is the palette section, for example "Actions". */
  group: string;
  icon?: LucideIcon;
  /** hint is quiet text at the right, for example an environment or a shortcut. */
  hint?: string;
  /** keywords are extra words that the search matches. */
  keywords?: string;
  /** when limits the command to the empty query ("idle") or to search results ("search"). */
  when?: "idle" | "search";
  run: () => void;
};

/**
 * matchScore scores a query against a text: 0 for no match, higher for a better match. A
 * substring beats a subsequence, and a match at a word start beats one inside a word.
 */
export function matchScore(query: string, text: string): number {
  const q = query.trim().toLowerCase();
  if (!q) return 1;
  const t = text.toLowerCase();
  const at = t.indexOf(q);
  if (at >= 0) {
    const wordStart = at === 0 || /[\s/._-]/.test(t[at - 1] ?? "");
    return 1000 - at + (wordStart ? 500 : 0) - t.length / 100;
  }
  let score = 0;
  let ti = 0;
  let run = 0;
  for (const ch of q) {
    if (ch === " ") continue;
    const found = t.indexOf(ch, ti);
    if (found < 0) return 0;
    run = found === ti ? run + 1 : 0;
    score += 1 + run * 2;
    ti = found + 1;
  }
  return score - t.length / 100;
}
