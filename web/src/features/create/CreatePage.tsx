import { useNavigate } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { createTrip } from "@/api/trips";
import type { CreatedTrip } from "@/api/types";
import { MyTripList } from "@/features/trips/MyTripList";
import { CreateForm } from "./CreateForm";
import { examples } from "./examples";

export function CreatePage() {
  const navigate = useNavigate();
  const mutation = useMutation({ mutationFn: createTrip });

  const created = (trip: CreatedTrip) => {
    navigate(`/trips/${trip.trip_id}`, { state: { runId: trip.run_id } });
  };

  return (
    <main className="mx-auto min-h-[calc(100dvh-3.5rem)] w-full max-w-6xl px-5 py-10 lg:py-16">
      <div className="grid items-center gap-10 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)] lg:gap-16">
        <header className="space-y-5">
          <p className="text-[11px] uppercase tracking-[0.2em] text-clay">北京 · AI 行程规划</p>
          <h1 className="font-serif text-4xl leading-[1.15] tracking-tight text-ink sm:text-5xl">
            用一句话，
            <br />
            规划你的北京行程
          </h1>
          <p className="max-w-md text-[15px] leading-relaxed text-muted-foreground">
            需求解析、候选筛选、时间调度与约束校验交给 Agent。你只需要描述想法，然后确认和微调。
          </p>
          <div className="max-w-md space-y-2 border-l-2 border-clay/35 pl-4">
            <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">试试这样说</p>
            <p className="text-sm leading-relaxed text-ink/80">“{examples[0].input}”</p>
            <p className="text-sm leading-relaxed text-muted-foreground">“{examples[1].input}”</p>
          </div>
        </header>

        <section
          aria-label="创建行程"
          className="rounded-lg border border-hairline bg-card p-5 shadow-sm sm:p-7"
        >
          <div className="mb-5 flex flex-wrap items-baseline justify-between gap-2">
            <h2 className="font-serif text-xl tracking-tight text-ink">描述你的行程</h2>
            <span className="text-xs text-muted-foreground">一句话即可，缺信息时会追问</span>
          </div>
          <CreateForm onSubmit={(request) => mutation.mutateAsync(request)} onCreated={created} />
        </section>
      </div>

      <div className="mt-12 max-w-3xl">
        <MyTripList />
      </div>
    </main>
  );
}
