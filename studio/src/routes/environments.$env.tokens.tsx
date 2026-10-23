import { createFileRoute } from "@tanstack/react-router";
import { TokensPage } from "@/features/settings/TokensPage";

export const Route = createFileRoute("/environments/$env/tokens")({
  component: function TokensRoute() {
    const { env } = Route.useParams();
    return <TokensPage env={env} />;
  },
});
