import { useQuery } from "@tanstack/react-query";
import { getTrip } from "@/api/trips";

export function useTrip(tripID: string | undefined) {
  return useQuery({
    queryKey: ["trip", tripID],
    queryFn: () => getTrip(tripID as string),
    enabled: Boolean(tripID),
    refetchInterval: (query) => {
      const status = query.state.data?.latest_run?.status;
      return status === "queued" || status === "running" ? 3000 : false;
    },
  });
}
