import { createFileRoute } from "@tanstack/react-router";
import { UsersPage } from "@/features/settings/UsersPage";

export const Route = createFileRoute("/settings/users")({
  component: UsersPage,
});
