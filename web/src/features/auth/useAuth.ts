import { useQuery } from "@tanstack/react-query";
import { getMe } from "@/api/auth";

export function useAuth() {
  return useQuery({
    queryKey: ["auth", "me"],
    queryFn: getMe,
    staleTime: 60_000,
    retry: false,
  });
}