import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { listTrips } from "@/api/trips";
import type { TripSummary } from "@/api/types";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateMessage";
import { StatusDot } from "@/design/primitives";
import { useAuth } from "@/features/auth/useAuth";

const statusLabels: Record<string, string> = {
  planning: "规划中",
  queued: "排队中",
  running: "进行中",
  awaiting_user: "待确认",
  completed: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

const statusTones: Record<string, "neutral" | "success" | "warning" | "danger" | "accent"> = {
  planning: "accent",
  queued: "neutral",
  running: "accent",
  awaiting_user: "warning",
  completed: "success",
  failed: "danger",
  cancelled: "neutral",
};

function cityLabel(city: string): string {
  return city === "beijing" || !city ? "北京" : city;
}

function formatDate(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return `${parsed.getMonth() + 1} 月 ${parsed.getDate()} 日`;
}

function TripRow({ trip }: { trip: TripSummary }) {
  const status = trip.latest_run?.status ?? trip.status;
  return (
    <li className="border-b border-hairline last:border-b-0">
      <Link
        to={`/trips/${trip.id}`}
        className="group flex min-h-14 items-center justify-between gap-4 py-3 transition-colors"
      >
        <div className="min-w-0 space-y-0.5">
          <p className="truncate font-serif text-base text-ink transition-colors group-hover:text-forest">
            {cityLabel(trip.city)} · {trip.days} 天
          </p>
          <p className="truncate text-xs tabular-nums text-muted-foreground">
            {formatDate(trip.created_at)}
            {trip.latest_version ? ` · 版本 v${trip.latest_version.seq}` : ""}
          </p>
        </div>
        <StatusDot
          tone={statusTones[status] ?? "neutral"}
          label={statusLabels[status] ?? status}
          className="shrink-0"
        />
      </Link>
    </li>
  );
}

export function MyTripList() {
  const auth = useAuth();
  const enabled = Boolean(auth.data?.enabled && auth.data.authenticated);
  const tripsQuery = useQuery({
    queryKey: ["trips", "list"],
    queryFn: () => listTrips(20),
    enabled,
    staleTime: 15_000,
    retry: false,
  });

  if (!enabled) {
    return null;
  }

  return (
    <section className="space-y-2" aria-labelledby="my-trips-title">
      <div className="flex items-baseline justify-between gap-2 border-b border-hairline pb-2">
        <h2 id="my-trips-title" className="font-serif text-lg tracking-tight text-ink">
          我的行程
        </h2>
        {tripsQuery.data && (
          <span className="text-xs tabular-nums text-muted-foreground">{tripsQuery.data.trips.length} 条</span>
        )}
      </div>

      {tripsQuery.isPending && <LoadingState label="正在加载行程…" rows={2} />}

      {tripsQuery.isError && (
        <ErrorState
          title="行程列表加载失败"
          description="服务暂时不可用，请稍后重试。"
          onRetry={() => void tripsQuery.refetch()}
        />
      )}

      {tripsQuery.data && tripsQuery.data.trips.length === 0 && (
        <EmptyState title="还没有行程" description="在上方描述你的北京行程需求，开始第一次规划。" />
      )}

      {tripsQuery.data && tripsQuery.data.trips.length > 0 && (
        <ul>
          {tripsQuery.data.trips.map((trip) => (
            <TripRow key={trip.id} trip={trip} />
          ))}
        </ul>
      )}
    </section>
  );
}
