export const roles = ["viewer", "editor", "owner"] as const;

export type Role = (typeof roles)[number];

export const roleLabels: Record<Role, string> = {
  viewer: "Viewer",
  editor: "Editor",
  owner: "Owner",
};

export function isRole(v: unknown): v is Role {
  return typeof v === "string" && (roles as readonly string[]).includes(v);
}

/** can reports whether role is at least min. An unknown role can do nothing. */
export function can(role: string | null | undefined, min: Role): boolean {
  if (!isRole(role)) return false;
  return roles.indexOf(role) >= roles.indexOf(min);
}
