import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function CreatePage() {
  return (
    <main className="mx-auto flex min-h-screen max-w-3xl flex-col justify-center gap-6 px-4 py-16">
      <div className="space-y-3">
        <p className="text-sm font-medium text-primary">北京 · 1–4 天</p>
        <h1 className="text-3xl font-semibold tracking-tight">用一句话规划你的北京行程</h1>
        <p className="text-muted-foreground">需求解析、候选筛选、时间调度与约束校验都由 Agent 完成，你只需要确认。</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>创建行程</CardTitle>
          <CardDescription>表单将在后续任务接入。</CardDescription>
        </CardHeader>
        <CardContent className="flex gap-3">
          <Button asChild>
            <Link to="/trips/demo">查看示例工作区</Link>
          </Button>
          <Button variant="outline" asChild>
            <Link to="/eval">评估面板</Link>
          </Button>
        </CardContent>
      </Card>
    </main>
  );
}
