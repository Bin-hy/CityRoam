import { Fragment, useEffect, useState, type FormEvent } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Play, RefreshCw } from "lucide-react";
import { loginURL } from "@/api/auth";
import { ApiError } from "@/api/client";
import { listEvalCases, listEvalRuns, startEval } from "@/api/eval";
import type { EvalRun } from "@/api/types";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useAuth } from "@/features/auth/useAuth";
import { useRunStream } from "@/features/eval/trace/useRunStream";
import { RunTrace } from "@/features/eval/trace/RunTrace";
import { aggregateMetrics, caseResults } from "./evalModel";

const TOKEN_KEY = "trippilot.adminToken";

function EvalRunTrace({ runID }: { runID: string }) {
  const trace = useRunStream(runID, { enabled: true });
  return (
    <div className="max-h-96 overflow-auto">
      <RunTrace state={trace} connected={trace.connected} />
    </div>
  );
}

function resultBadge(run: EvalRun) {
  return run.passed ? <Badge variant="success">通过</Badge> : <Badge variant="destructive">未通过</Badge>;
}

export function EvalDashboard() {
  const auth = useAuth();
  const [token, setToken] = useState(() => sessionStorage.getItem(TOKEN_KEY) ?? "");
  const [draft, setDraft] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);
  const [polling, setPolling] = useState(false);

  const sessionAdmin = auth.data?.enabled === true && auth.data.authenticated === true && auth.data.user?.role === "admin";
  const needsAdminRole = auth.data?.enabled === true && auth.data.authenticated === true && !sessionAdmin;
  const usingToken = !sessionAdmin;
  const activeToken = usingToken ? token : "";

  useEffect(() => {
    if (!polling) return;
    const timer = setTimeout(() => setPolling(false), 120_000);
    return () => clearTimeout(timer);
  }, [polling]);

  const casesQuery = useQuery({
    queryKey: ["eval", "cases", activeToken],
    queryFn: () => listEvalCases(activeToken),
    enabled: !auth.isPending && !needsAdminRole && (sessionAdmin || Boolean(token)),
    retry: false,
  });
  const runsQuery = useQuery({
    queryKey: ["eval", "runs", activeToken],
    queryFn: () => listEvalRuns(activeToken),
    enabled: !auth.isPending && !needsAdminRole && (sessionAdmin || Boolean(token)),
    retry: false,
    refetchInterval: polling ? 5_000 : false,
  });
  const start = useMutation({
    mutationFn: () => startEval(activeToken),
    onSuccess: () => setPolling(true),
  });

  function enterToken(event: FormEvent) {
    event.preventDefault();
    const next = draft.trim();
    if (!next) return;
    sessionStorage.setItem(TOKEN_KEY, next);
    setToken(next);
    setDraft("");
  }

  function resetToken() {
    sessionStorage.removeItem(TOKEN_KEY);
    setToken("");
    setDraft("");
    setExpanded(null);
  }

  if (auth.isPending) {
    return (
      <main className="mx-auto max-w-md px-4 py-16">
        <Card>
          <CardContent className="pt-6 text-sm text-muted-foreground">正在确认登录状态…</CardContent>
        </Card>
      </main>
    );
  }

  if (needsAdminRole) {
    return (
      <main className="mx-auto max-w-md px-4 py-16">
        <Card>
          <CardHeader>
            <CardTitle className="font-serif tracking-tight">评估面板</CardTitle>
            <CardDescription>评估面板仅对管理员开放。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-sm text-muted-foreground">
            <p>当前账号没有管理员角色。请联系管理员将其加入白名单，或使用管理员令牌访问。</p>
            <Button size="sm" variant="outline" asChild>
              <Link to="/">返回首页</Link>
            </Button>
          </CardContent>
        </Card>
      </main>
    );
  }

  if (usingToken && !token) {
    return (
      <main className="mx-auto max-w-md px-4 py-16">
        <Card>
          <CardHeader>
            <CardTitle className="font-serif tracking-tight">评估面板</CardTitle>
            <CardDescription>请输入管理员令牌以查看评测用例与结果。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-3" onSubmit={enterToken}>
              <label className="grid gap-1 text-sm">
                <span className="text-muted-foreground">管理员令牌</span>
                <input
                  type="password"
                  aria-label="管理员令牌"
                  className="h-9 rounded-md border border-hairline bg-paper px-2 text-sm"
                  value={draft}
                  onChange={(event) => setDraft(event.target.value)}
                  autoComplete="off"
                />
              </label>
              <Button type="submit" disabled={!draft.trim()}>
                进入评测面板
              </Button>
              <p className="text-xs text-muted-foreground">令牌只保存在当前浏览器会话中，不会写入服务端。</p>
            </form>
          </CardContent>
        </Card>
      </main>
    );
  }

  const error = casesQuery.error ?? runsQuery.error;
  const unauthorized = error instanceof ApiError && error.status === 401;
  const unavailable = error instanceof ApiError && error.status === 503;
  const cases = casesQuery.data?.cases ?? [];
  const runs = runsQuery.data?.runs ?? [];
  const rows = caseResults(cases, runs);
  const metrics = aggregateMetrics(runs);

  return (
    <main className="mx-auto max-w-6xl space-y-4 px-4 py-8">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="font-serif text-lg tracking-tight text-ink">评估面板</h1>
          <p className="text-sm text-muted-foreground">
            {cases.length} 条用例 · {runs.length} 次运行
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" onClick={() => void Promise.all([casesQuery.refetch(), runsQuery.refetch()])}>
            <RefreshCw className="mr-1 h-4 w-4" />
            刷新结果
          </Button>
          <Button size="sm" onClick={() => start.mutate()} disabled={start.isPending || unavailable}>
            <Play className="mr-1 h-4 w-4" />
            {start.isPending ? "启动中…" : "启动评测"}
          </Button>
          {usingToken && (
            <Button size="sm" variant="ghost" onClick={resetToken}>
              退出
            </Button>
          )}
        </div>
      </header>

      {start.isSuccess && (
        <Alert className="text-sm">评测已启动，结果会自动刷新；也可手动刷新查看最新进度。</Alert>
      )}
      {start.isError && (
        <Alert variant="destructive" className="text-sm">
          启动评测失败，请稍后重试。
        </Alert>
      )}
      {unauthorized && (
        <Alert variant="destructive" className="flex flex-wrap items-center justify-between gap-2 text-sm">
          <span>{sessionAdmin ? "会话已过期，请重新登录。" : "管理员令牌无效。"}</span>
          {sessionAdmin ? (
            <Button size="sm" variant="outline" asChild>
              <a href={loginURL("/eval")}>重新登录</a>
            </Button>
          ) : (
            <Button size="sm" variant="outline" onClick={resetToken}>
              重新输入
            </Button>
          )}
        </Alert>
      )}
      {unavailable && !unauthorized && (
        <Alert className="text-sm">当前部署未启用评测服务，请联系管理员完成评测运行器配置。</Alert>
      )}
      {error && !unauthorized && !unavailable && (
        <Alert variant="destructive" className="text-sm">
          评测数据加载失败，请重试。
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="font-serif tracking-tight">指标汇总</CardTitle>
          <CardDescription>基于全部已记录运行的平均值，缺失指标显示为「—」。</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-muted-foreground">
                  <th className="py-2 pr-4 font-medium">指标</th>
                  <th className="py-2 pr-4 font-medium">数值</th>
                  <th className="py-2 font-medium">样本</th>
                </tr>
              </thead>
              <tbody>
                {metrics.map((metric) => (
                  <tr key={metric.key} className="border-b last:border-0">
                    <td className="py-2 pr-4">{metric.label}</td>
                    <td className="py-2 pr-4 font-medium">{metric.value}</td>
                    <td className="py-2 text-muted-foreground">{metric.samples}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-serif tracking-tight">用例与最近结果</CardTitle>
          <CardDescription>失败用例可展开 Agent trace，定位约束、修复与工具调用。</CardDescription>
        </CardHeader>
        <CardContent>
          {(casesQuery.isPending || runsQuery.isPending) && <Skeleton className="h-40" />}
          {!casesQuery.isPending && !runsQuery.isPending && rows.length === 0 && !error && (
            <p className="text-sm text-muted-foreground">暂无评测用例。</p>
          )}
          {rows.length > 0 && (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="py-2 pr-4 font-medium">用例</th>
                    <th className="py-2 pr-4 font-medium">标签</th>
                    <th className="py-2 pr-4 font-medium">状态</th>
                    <th className="py-2 pr-4 font-medium">最近结果</th>
                    <th className="py-2 pr-4 font-medium">延迟</th>
                    <th className="py-2 font-medium">Trace</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map(({ testCase, latest }) => (
                    <Fragment key={testCase.id}>
                      <tr className="border-b align-top last:border-0">
                        <td className="max-w-md py-2 pr-4">
                          <p className="font-medium">{testCase.id}</p>
                          <p className="text-xs text-muted-foreground">{testCase.input}</p>
                        </td>
                        <td className="py-2 pr-4">
                          <div className="flex flex-wrap gap-1">
                            {testCase.tags.map((tag) => (
                              <Badge key={tag} variant="outline">
                                {tag}
                              </Badge>
                            ))}
                          </div>
                        </td>
                        <td className="py-2 pr-4 text-muted-foreground">{testCase.enabled ? "启用" : "停用"}</td>
                        <td className="py-2 pr-4">{latest ? resultBadge(latest) : <span className="text-muted-foreground">未运行</span>}</td>
                        <td className="py-2 pr-4 text-muted-foreground">
                          {latest?.duration_ms != null ? `${latest.duration_ms} ms` : "—"}
                        </td>
                        <td className="py-2">
                          {latest && !latest.passed && latest.run_id ? (
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => setExpanded(expanded === latest.run_id ? null : (latest.run_id ?? null))}
                            >
                              {expanded === latest.run_id ? "收起 trace" : "查看 trace"}
                            </Button>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </td>
                      </tr>
                      {latest?.run_id && expanded === latest.run_id && (
                        <tr className="border-b last:border-0">
                          <td colSpan={6} className="bg-sand/40 p-3">
                            <EvalRunTrace runID={latest.run_id} />
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </main>
  );
}
