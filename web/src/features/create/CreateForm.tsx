import { useRef, useState, type FormEvent } from "react";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/api/client";
import type { CreatedTrip, CreateTripRequest } from "@/api/types";
import { newClientRequestID } from "@/lib/id";
import { composeInput, countAdvanced, emptyAdvanced, type AdvancedOptions } from "./composeInput";
import { examples } from "./examples";

const MAX_INPUT = 2000;

interface CreateFormProps {
  onSubmit: (request: CreateTripRequest) => Promise<CreatedTrip>;
  onCreated?: (trip: CreatedTrip) => void;
}

export function CreateForm({ onSubmit, onCreated }: CreateFormProps) {
  const [text, setText] = useState("");
  const [advanced, setAdvanced] = useState<AdvancedOptions>(emptyAdvanced);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestID = useRef("");
  const composed = composeInput(text, advanced);

  const changeText = (value: string) => {
    requestID.current = "";
    setText(value);
  };

  const changeAdvanced = (patch: Partial<AdvancedOptions>) => {
    requestID.current = "";
    setAdvanced((current) => ({ ...current, ...patch }));
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!composed.trim()) {
      setError("请先描述你的北京行程。");
      return;
    }
    if (composed.length > MAX_INPUT) {
      setError(`需求描述不能超过 ${MAX_INPUT} 字。`);
      return;
    }
    if (!requestID.current) {
      requestID.current = newClientRequestID();
    }
    setSubmitting(true);
    setError(null);
    try {
      const trip = await onSubmit({
        input: composed,
        city: "beijing",
        client_request_id: requestID.current,
      });
      requestID.current = "";
      onCreated?.(trip);
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "提交失败，请稍后再试。");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>描述你的行程</CardTitle>
        <CardDescription>一句话即可，Agent 会解析硬/软约束并在需要时追问。</CardDescription>
      </CardHeader>
      <CardContent>
        <form className="space-y-4" onSubmit={submit}>
          <div className="space-y-2">
            <Textarea
              aria-label="行程需求"
              value={text}
              maxLength={MAX_INPUT}
              placeholder="例如：北京两天，想看历史和博物馆，预算 1000 元以内，节奏轻松一点。"
              onChange={(event) => changeText(event.target.value)}
            />
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>支持北京 1–4 天行程</span>
              <span>
                {composed.length}/{MAX_INPUT}
              </span>
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            {examples.map((example) => (
              <Button
                key={example.label}
                type="button"
                variant="secondary"
                size="sm"
                onClick={() => changeText(example.input)}
              >
                {example.label}
              </Button>
            ))}
          </div>

          <div className="rounded-lg border">
            <button
              type="button"
              className="flex w-full items-center justify-between px-4 py-3 text-left text-sm font-medium"
              onClick={() => setShowAdvanced((open) => !open)}
              aria-expanded={showAdvanced}
            >
              <span>高级选项</span>
              <span className="text-xs text-muted-foreground">
                {countAdvanced(advanced) > 0 ? `已设置 ${countAdvanced(advanced)} 项` : "可选"}
              </span>
            </button>
            {showAdvanced && (
              <div className="grid gap-3 border-t p-4 sm:grid-cols-2">
                <Field label="天数">
                  <select
                    aria-label="天数"
                    className="h-10 w-full rounded-md border border-input bg-card px-3 text-sm"
                    value={advanced.days}
                    onChange={(event) => changeAdvanced({ days: event.target.value })}
                  >
                    <option value="">不限（由描述决定）</option>
                    {[1, 2, 3, 4].map((day) => (
                      <option key={day} value={day}>
                        {day} 天
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="节奏">
                  <select
                    aria-label="节奏"
                    className="h-10 w-full rounded-md border border-input bg-card px-3 text-sm"
                    value={advanced.pace}
                    onChange={(event) => changeAdvanced({ pace: event.target.value })}
                  >
                    <option value="">默认</option>
                    <option value="relaxed">轻松</option>
                    <option value="normal">适中</option>
                    <option value="packed">紧凑</option>
                  </select>
                </Field>
                <Field label="同行人数">
                  <Input
                    aria-label="同行人数"
                    type="number"
                    min={1}
                    max={10}
                    value={advanced.partySize}
                    onChange={(event) => changeAdvanced({ partySize: event.target.value })}
                  />
                </Field>
                <Field label="总预算（元）">
                  <Input
                    aria-label="总预算"
                    type="number"
                    min={0}
                    step={100}
                    value={advanced.budgetYuan}
                    onChange={(event) => changeAdvanced({ budgetYuan: event.target.value })}
                  />
                </Field>
                <Field label="每日游玩上限（小时）">
                  <Input
                    aria-label="每日游玩上限"
                    type="number"
                    min={1}
                    max={16}
                    value={advanced.maxHours}
                    onChange={(event) => changeAdvanced({ maxHours: event.target.value })}
                  />
                </Field>
                <Field label="感兴趣">
                  <Input
                    aria-label="感兴趣"
                    placeholder="历史、咖啡、夜景"
                    value={advanced.interests}
                    onChange={(event) => changeAdvanced({ interests: event.target.value })}
                  />
                </Field>
                <Field label="必须去">
                  <Input
                    aria-label="必须去"
                    placeholder="故宫、天坛"
                    value={advanced.mustVisit}
                    onChange={(event) => changeAdvanced({ mustVisit: event.target.value })}
                  />
                </Field>
                <Field label="避开">
                  <Input
                    aria-label="避开"
                    placeholder="人多、爬山、辣"
                    value={advanced.avoid}
                    onChange={(event) => changeAdvanced({ avoid: event.target.value })}
                  />
                </Field>
                <label className="flex items-center gap-2 text-sm sm:col-span-2">
                  <input
                    type="checkbox"
                    checked={advanced.meals}
                    onChange={(event) => changeAdvanced({ meals: event.target.checked })}
                  />
                  安排午餐和晚餐
                </label>
              </div>
            )}
          </div>

          {error && <Alert variant="destructive">{error}</Alert>}

          <div className="flex items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">AI 生成，价格与时间为估算，出行前请核实。</p>
            <Button type="submit" disabled={submitting}>
              {submitting ? "规划中…" : "开始规划"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="space-y-1 text-sm">
      <span className="text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}
