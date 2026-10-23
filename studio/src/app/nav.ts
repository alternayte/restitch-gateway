import { FileCode2, Gauge, History, KeyRound, Layers, Users, type LucideIcon } from "lucide-react";
import type { Role } from "@/lib/roles";

/** NavItem is one page of the side nav. chord is the second key of its "g" shortcut. */
export type NavItem = { to: string; label: string; min: Role; icon: LucideIcon; chord?: string };

export const navItems: NavItem[] = [{ to: "/", label: "Environments", min: "viewer", icon: Layers, chord: "e" }];

export const settingsItems: NavItem[] = [{ to: "/settings/users", label: "Users", min: "owner", icon: Users, chord: "u" }];

/** EnvPage is one page of an environment. Its "g" chord works while an environment page is open. */
export type EnvPage = {
  to: "/environments/$env" | "/environments/$env/draft" | "/environments/$env/versions" | "/environments/$env/tokens";
  label: string;
  min: Role;
  icon: LucideIcon;
  chord: string;
  exact: boolean;
  keywords?: string;
};

export const envPages: EnvPage[] = [
  { to: "/environments/$env", label: "Overview", min: "viewer", icon: Gauge, chord: "o", exact: true, keywords: "dashboard usage requests instances" },
  { to: "/environments/$env/draft", label: "Draft", min: "viewer", icon: FileCode2, chord: "d", exact: false, keywords: "resources" },
  { to: "/environments/$env/versions", label: "Publish and history", min: "viewer", icon: History, chord: "p", exact: true, keywords: "publish versions rollback promote diff" },
  { to: "/environments/$env/tokens", label: "Gateway tokens", min: "owner", icon: KeyRound, chord: "t", exact: true, keywords: "token" },
];

/** currentEnv returns the environment of an environment page path, or undefined. */
export function currentEnv(pathname: string): string | undefined {
  const m = /^\/environments\/([^/]+)/.exec(pathname);
  return m?.[1] ? decodeURIComponent(m[1]) : undefined;
}
