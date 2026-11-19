import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function EvalPage() {
  return (
    <main className="mx-auto max-w-6xl px-4 py-10">
      <Card>
        <CardHeader>
          <CardTitle>评估面板</CardTitle>
          <CardDescription>用例与运行指标将在后续任务接入。</CardDescription>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">需要管理员令牌访问。</CardContent>
      </Card>
    </main>
  );
}
