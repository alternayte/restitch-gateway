import type { Resource } from "@/api/types.gen";

export type Kind = Resource["kind"];

export const kinds: { kind: Kind; label: string; singleton: boolean }[] = [
  { kind: "upstream", label: "Upstream", singleton: false },
  { kind: "composition", label: "Composition", singleton: false },
  { kind: "auth", label: "Auth policy", singleton: true },
  { kind: "rate_limit", label: "Rate limit", singleton: true },
  { kind: "settings", label: "Settings", singleton: true },
];

export const kindLabel = (k: Kind) => kinds.find((x) => x.kind === k)?.label ?? k;
