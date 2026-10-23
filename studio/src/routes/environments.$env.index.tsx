import { createFileRoute } from "@tanstack/react-router";
import { OverviewPage } from "@/features/dashboard/OverviewPage";

export const Route = createFileRoute("/environments/$env/")({
  component: function OverviewRoute() {
    const { env } = Route.useParams();
    return <OverviewPage env={env} />;
  },
});
