import { useParams } from "react-router-dom";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function SharePage() {
  const { slug } = useParams();
  return (
    <main className="mx-auto max-w-4xl px-4 py-10">
      <Card>
        <CardHeader>
          <CardTitle>分享的行程</CardTitle>
          <CardDescription>slug: {slug ?? "未知"}</CardDescription>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">只读行程页面将在后续任务接入。</CardContent>
      </Card>
    </main>
  );
}
