import { createFileRoute } from "@tanstack/react-router";
import { VersionsPage } from "@/features/publish/VersionsPage";

export const Route = createFileRoute("/environments/$env/versions")({
  component: function VersionsRoute() {
    const { env } = Route.useParams();
    return <VersionsPage env={env} />;
  },
});
