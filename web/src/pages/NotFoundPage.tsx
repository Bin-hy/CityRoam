import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";

export function NotFoundPage() {
  return (
    <main className="mx-auto flex min-h-screen max-w-xl flex-col items-center justify-center gap-4 px-4 text-center">
      <h1 className="text-2xl font-semibold">页面不存在</h1>
      <p className="text-muted-foreground">链接可能已失效。</p>
      <Button asChild>
        <Link to="/">返回首页</Link>
      </Button>
    </main>
  );
}
