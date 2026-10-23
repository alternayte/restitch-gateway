import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import { signOut as signOutRequest } from "@/api/sdk.gen";
import { getMeOptions, getMeQueryKey } from "@/api/@tanstack/react-query.gen";
import type { MeOutputBody } from "@/api/types.gen";

export type Me = MeOutputBody;

/** meQueryKey builds the query key at call time. Build it after api-client sets the base URL. */
export const meQueryKey = () => getMeQueryKey();

/** useMe loads the current user. A 401 error means "not signed in". */
export function useMe() {
  return useQuery({ ...getMeOptions(), retry: false, staleTime: 60_000 });
}

/** useSignOut ends the session, clears the cache and goes to the sign-in page. */
export function useSignOut() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const signOut = useCallback(async () => {
    setBusy(true);
    try {
      await signOutRequest();
    } finally {
      qc.clear();
      setBusy(false);
      void navigate({ to: "/login" });
    }
  }, [qc, navigate]);
  return { signOut, busy };
}

/** useCurrentUser returns the signed-in user. Use it only inside the app shell. */
export function useCurrentUser(): Me {
  const { data } = useMe();
  if (!data) throw new Error("The current user is not loaded.");
  return data;
}
