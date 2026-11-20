import { useRef, useState, type FormEvent } from "react";
import { SlidersHorizontal } from "lucide-react";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/api/client";
import type { CreatedTrip, CreateTripRequest } from "@/api/types";
import { newClientRequestID } from "@/lib/id";
import { composeInput, countAdvanced, emptyAdvanced, type AdvancedOptions } from "./composeInput";
import { examples } from "./examples";
import { PreferenceSheet } from "./PreferenceSheet";

const MAX_INPUT = 2000;

interface CreateFormProps {
  onSubmit: (request: CreateTripRequest) => Promise<CreatedTrip>;
  onCreated?: (trip: CreatedTrip) => void;
}

export function CreateForm({ onSubmit, onCreated }: CreateFormProps) {
  const [text, setText] = useState("");
  const [advanced, setAdvanced] = useState<AdvancedOptions>(emptyAdvanced);
  const [preferencesOpen, setPreferencesOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestID = useRef("");
  const composed = composeInput(text, advanced);
  const advancedCount = countAdvanced(advanced);

  const changeText = (value: string) => {
    requestID.current = "";
    setText(value);
  };

  const changeAdvanced = (patch: Partial<AdvancedOptions>) => {
    requestID.current = "";
    setAdvanced((current) => ({ ...current, ...patch }));
  };

  const resetAdvanced = () => {
    requestID.current = "";
    setAdvanced(emptyAdvanced);
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
    <>
      <form className="space-y-4" onSubmit={submit}>
        <div className="rounded-md border border-hairline bg-paper/60 transition-colors focus-within:border-forest/50">
          <Textarea
            aria-label="行程需求"
            value={text}
            maxLength={MAX_INPUT}
            rows={5}
            placeholder="例如：北京两天，想看历史和博物馆，预算 1000 元以内，节奏轻松一点。"
            className="min-h-40 resize-none border-0 bg-transparent p-4 text-[15px] leading-relaxed shadow-none focus-visible:ring-0"
            onChange={(event) => changeText(event.target.value)}
          />
          <div className="flex items-center justify-between gap-3 px-4 pb-3 text-[11px] tabular-nums text-muted-foreground">
            <span>支持北京 1–4 天行程</span>
            <span>
              {composed.length}/{MAX_INPUT}
            </span>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <span className="mr-1 text-[11px] uppercase tracking-[0.16em] text-muted-foreground">示例</span>
          {examples.map((example) => (
            <button
              key={example.label}
              type="button"
              onClick={() => changeText(example.input)}
              className="min-h-9 rounded-full border border-hairline bg-paper px-3 text-xs text-ink/80 transition-colors hover:border-forest/40 hover:text-ink"
            >
              {example.label}
            </button>
          ))}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setPreferencesOpen(true)}
          >
            <SlidersHorizontal className="mr-1 h-3.5 w-3.5" aria-hidden />
            偏好{advancedCount > 0 ? ` · ${advancedCount} 项` : ""}
          </Button>
        </div>

        {error && <Alert variant="destructive">{error}</Alert>}

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-hairline pt-4">
          <p className="max-w-[18rem] text-xs leading-relaxed text-muted-foreground">
            AI 生成，价格与时间为估算，出行前请核实。
          </p>
          <Button type="submit" size="lg" disabled={submitting}>
            {submitting ? "规划中…" : "开始规划"}
          </Button>
        </div>
      </form>

      <PreferenceSheet
        open={preferencesOpen}
        onOpenChange={setPreferencesOpen}
        options={advanced}
        onChange={changeAdvanced}
        onReset={resetAdvanced}
      />
    </>
  );
}
