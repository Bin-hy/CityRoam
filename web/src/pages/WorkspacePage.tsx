import { useParams } from "react-router-dom";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function WorkspacePage() {
  const { tripId } = useParams();
  return (
    <main className="mx-auto max-w-6xl px-4 py-10">
      <Card>
        <CardHeader>
          <CardTitle>行程工作区</CardTitle>
          <CardDescription>trip id: {tripId ?? "未知"}</CardDescription>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">三栏工作区将在后续任务接入。</CardContent>
      </Card>
    </main>
  );
}
