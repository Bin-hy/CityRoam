import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { LogOut, Map } from "lucide-react";
import { logout } from "@/api/auth";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/features/auth/useAuth";

export function AppHeader() {
  const auth = useAuth();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: (data) => {
      queryClient.clear();
      if (data && "redirect_url" in data && data.redirect_url) {
        window.location.assign(data.redirect_url);
        return;
      }
      navigate("/login", { replace: true });
    },
  });

  if (!auth.data?.enabled || !auth.data.authenticated) {
    return null;
  }

  const user = auth.data.user;
  const label = user?.name || user?.email || "已登录用户";

  return (
    <header className="flex h-14 flex-wrap items-center justify-between gap-3 border-b border-hairline bg-paper/95 px-5 backdrop-blur">
      <Link to="/" className="flex items-center gap-2 font-serif text-[17px] tracking-tight text-ink">
        <Map className="h-4 w-4 text-forest" />
        TripPilot
      </Link>
      <div className="flex items-center gap-3 text-sm">
        <span className="max-w-[16rem] truncate text-muted-foreground" title={user?.email ?? undefined}>
          {label}
        </span>
        {user?.role === "admin" && (
          <Badge variant="outline" className="border-hairline text-clay">
            管理员
          </Badge>
        )}
        <Button
          size="sm"
          variant="ghost"
          className="text-muted-foreground hover:text-ink"
          onClick={() => signOut.mutate()}
          disabled={signOut.isPending}
          aria-label="退出登录"
        >
          <LogOut className="h-4 w-4" />
          {signOut.isPending ? "退出中…" : "退出"}
        </Button>
      </div>
    </header>
  );
}