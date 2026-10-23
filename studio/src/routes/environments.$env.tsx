import { createFileRoute, Outlet } from "@tanstack/react-router";
import { EnvironmentNav } from "@/features/dashboard/EnvironmentNav";

export const Route = createFileRoute("/environments/$env")({
  component: function EnvironmentLayout() {
    const { env } = Route.useParams();
    return (
      <>
        <EnvironmentNav env={env} />
        <Outlet />
      </>
    );
  },
});
