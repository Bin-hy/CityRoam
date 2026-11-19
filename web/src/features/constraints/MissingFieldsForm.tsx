import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

interface MissingFieldsFormProps {
  missing: string[];
  onSubmit: (supplement: string) => void;
  submitting?: boolean;
}

export function MissingFieldsForm({ missing, onSubmit, submitting = false }: MissingFieldsFormProps) {
  const [city, setCity] = useState("beijing");
  const [days, setDays] = useState("2");
  const daysValue = Number.parseInt(days, 10);
  const daysValid = Number.isInteger(daysValue) && daysValue >= 1 && daysValue <= 4;
  const needsCity = missing.includes("city");
  const needsDays = missing.includes("days");

  if (!needsCity && !needsDays) return null;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (needsDays && !daysValid) return;
    const parts: string[] = [];
    if (needsCity) parts.push(`城市是${city === "beijing" ? "北京" : city}`);
    if (needsDays) parts.push(`共 ${daysValue} 天`);
    onSubmit(parts.join("，"));
  };

  return (
    <form className="space-y-3 rounded-lg border border-amber-200 bg-amber-50 p-4" onSubmit={submit}>
      <div>
        <h3 className="text-sm font-semibold text-amber-900">还差一点信息</h3>
        <p className="text-xs text-amber-800">补充后 Agent 会继续规划。</p>
      </div>
      <div className="flex flex-wrap gap-3">
        {needsCity && (
          <label className="flex items-center gap-2 text-sm">
            城市
            <select
              className="h-9 rounded-md border border-input bg-card px-2 text-sm"
              value={city}
              onChange={(event) => setCity(event.target.value)}
            >
              <option value="beijing">北京</option>
            </select>
          </label>
        )}
        {needsDays && (
          <label className="flex items-center gap-2 text-sm">
            天数
            <Input
              aria-label="天数"
              type="number"
              min={1}
              max={4}
              value={days}
              onChange={(event) => setDays(event.target.value)}
              className="h-9 w-20"
            />
          </label>
        )}
      </div>
      {needsDays && !daysValid && <p className="text-xs text-destructive">天数需在 1–4 之间。</p>}
      <Button type="submit" size="sm" disabled={submitting || (needsDays && !daysValid)}>
        {submitting ? "提交中…" : "补充并继续"}
      </Button>
    </form>
  );
}
