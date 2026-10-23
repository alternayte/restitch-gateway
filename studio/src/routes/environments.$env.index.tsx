import { createFileRoute } from "@tanstack/react-router";
import { DraftPage } from "@/features/resources/DraftPage";

export const Route = createFileRoute("/environments/$env/")({
  component: function DraftRoute() {
    const { env } = Route.useParams();
    return <DraftPage env={env} />;
  },
});
