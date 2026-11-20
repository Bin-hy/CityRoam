import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { Loader2 } from "lucide-react";
import { ErrorState } from "@/components/StateMessage";
import { useAuth } from "./useAuth";

export function loginRedirectTarget(pathname: string, search = ""): string {
  const returnTo = `${pathname}${search}`;
  return `/login?return_to=${encodeURIComponent(returnTo)}`;
}

export function AuthGate({ children }: { children: ReactNode }) {
  const auth = useAuth();
  const location = useLocation();

  if (auth.isPending) {
    return (
      <main className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        正在检查登录状态…
      </main>
    );
  }

  if (auth.isError) {
    return (
      <main className="mx-auto max-w-xl px-4 py-16">
        <ErrorState
          title="无法确认登录状态"
          description="请检查网络或稍后重试。"
          onRetry={() => void auth.refetch()}
        />
      </main>
    );
  }

  if (!auth.data.enabled || auth.data.authenticated) {
    return <>{children}</>;
  }

  return <Navigate to={loginRedirectTarget(location.pathname, location.search)} replace />;
}