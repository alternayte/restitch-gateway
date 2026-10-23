import { Link } from "@tanstack/react-router";
import { envPages } from "@/app/nav";
import { useCurrentUser } from "@/lib/auth";
import { can } from "@/lib/roles";

/** EnvironmentNav switches between the pages of one environment. */
export function EnvironmentNav({ env }: { env: string }) {
  const me = useCurrentUser();
  return (
    <nav aria-label={`Environment ${env}`} className="-mt-1 mb-5 flex flex-wrap gap-1 border-b">
      {envPages
        .filter((item) => can(me.role, item.min))
        .map((item) => (
          <Link
            key={item.to}
            to={item.to}
            params={{ env }}
            activeOptions={{ exact: item.exact }}
            className="-mb-px border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground hover:text-foreground data-[status=active]:border-accent data-[status=active]:text-foreground"
          >
            {item.label}
          </Link>
        ))}
    </nav>
  );
}
