import { Link, Navigate, useSearchParams } from "react-router-dom";
import { LogIn, Map } from "lucide-react";
import { loginURL } from "@/api/auth";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useAuth } from "./useAuth";

const errorMessages: Record<string, string> = {
  auth_failed: "登录失败，请重试。",
  login_state_invalid: "登录状态已失效，请重新发起登录。",
  oidc_error: "身份提供方返回了错误，请重试。",
  access_denied: "你取消了授权，如需使用请重新登录。",
  callback_failed: "登录回调处理失败，请重试。",
};

export function loginErrorMessage(code: string | null): string | null {
  if (!code) return null;
  return errorMessages[code] ?? "登录未完成，请重试。";
}

export function sanitizeReturnTo(raw: string | null): string {
  if (!raw || !raw.startsWith("/") || raw.startsWith("//") || raw.includes("\\")) {
    return "/";
  }
  return raw;
}

export function LoginPage() {
  const auth = useAuth();
  const [params] = useSearchParams();
  const returnTo = sanitizeReturnTo(params.get("return_to"));
  const message = loginErrorMessage(params.get("error"));

  if (auth.isPending) {
    return (
      <main className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">
        正在检查登录状态…
      </main>
    );
  }

  if (auth.data?.authenticated) {
    return <Navigate to={returnTo} replace />;
  }

  if (auth.data && !auth.data.enabled) {
    return <Navigate to={returnTo} replace />;
  }

  const provider = auth.data?.provider_name || "统一登录";

  return (
    <main className="flex min-h-screen flex-col bg-paper">
      <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col justify-center px-4 py-16 lg:px-8">
        <div className="max-w-md space-y-7">
          <Link
            to="/"
            className="inline-flex items-center gap-2 font-serif text-lg tracking-tight text-ink transition-colors hover:text-forest"
          >
            <Map className="h-4 w-4 text-forest" />
            TripPilot
          </Link>

          <div className="space-y-2">
            <p className="text-[11px] uppercase tracking-[0.2em] text-muted-foreground">登录</p>
            <h1 className="font-serif text-3xl leading-tight tracking-tight text-ink">登录 TripPilot</h1>
            <p className="text-sm leading-relaxed text-muted-foreground">
              使用 {provider} 验证身份。登录后会回到你刚才访问的页面。
            </p>
          </div>

          {message && (
            <Alert variant="destructive" className="text-sm">
              {message}
            </Alert>
          )}

          <Button size="lg" className="w-full" asChild>
            <a href={loginURL(returnTo)}>
              <LogIn className="h-4 w-4" />
              使用 {provider} 登录
            </a>
          </Button>

          <p className="text-xs text-muted-foreground">分享链接无需登录即可查看。</p>
        </div>
      </div>
    </main>
  );
}
