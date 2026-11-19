import { useNavigate } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { createTrip } from "@/api/trips";
import type { CreatedTrip } from "@/api/types";
import { CreateForm } from "./CreateForm";

export function CreatePage() {
  const navigate = useNavigate();
  const mutation = useMutation({ mutationFn: createTrip });

  const created = (trip: CreatedTrip) => {
    navigate(`/trips/${trip.trip_id}`, { state: { runId: trip.run_id } });
  };

  return (
    <main className="mx-auto flex min-h-screen max-w-3xl flex-col justify-center gap-8 px-4 py-16">
      <header className="space-y-3">
        <p className="text-sm font-medium text-primary">北京 · 1–4 天 · AI 行程规划</p>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">用一句话规划你的北京行程</h1>
        <p className="text-muted-foreground">
          需求解析、候选筛选、时间调度与约束校验交给 Agent，你只需要确认和微调。
        </p>
      </header>
      <CreateForm onSubmit={(request) => mutation.mutateAsync(request)} onCreated={created} />
    </main>
  );
}
