import { Layers, Users, type LucideIcon } from "lucide-react";
import type { Role } from "@/lib/roles";

/** NavItem is one page of the side nav. chord is the second key of its "g" shortcut. */
export type NavItem = { to: string; label: string; min: Role; icon: LucideIcon; chord?: string };

export const navItems: NavItem[] = [{ to: "/", label: "Environments", min: "viewer", icon: Layers, chord: "e" }];

export const settingsItems: NavItem[] = [{ to: "/settings/users", label: "Users", min: "owner", icon: Users, chord: "u" }];
