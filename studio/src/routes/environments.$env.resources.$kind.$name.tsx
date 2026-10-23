import { createFileRoute, notFound } from "@tanstack/react-router";
import { ResourceEditor } from "@/features/resources/ResourceEditor";
import { kinds, type Kind } from "@/features/resources/kinds";

export const Route = createFileRoute("/environments/$env/resources/$kind/$name")({
  beforeLoad: ({ params }) => {
    if (!kinds.some((k) => k.kind === params.kind)) throw notFound();
  },
  component: function ResourceRoute() {
    const { env, kind, name } = Route.useParams();
    return <ResourceEditor key={`${env}/${kind}/${name}`} env={env} kind={kind as Kind} name={name} />;
  },
});
