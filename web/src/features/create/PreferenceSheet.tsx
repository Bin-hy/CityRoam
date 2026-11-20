import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { countAdvanced, type AdvancedOptions } from "./composeInput";

export interface PreferenceSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  options: AdvancedOptions;
  onChange: (patch: Partial<AdvancedOptions>) => void;
  onReset: () => void;
}

export function PreferenceSheet({ open, onOpenChange, options, onChange, onReset }: PreferenceSheetProps) {
  const count = countAdvanced(options);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85dvh] flex-col overflow-hidden p-0">
        <DialogHeader className="mb-0 border-b border-hairline px-5 py-4">
          <DialogTitle className="font-serif">行程偏好</DialogTitle>
          <DialogDescription>可选，补充后会自动附加到你的描述里。</DialogDescription>
        </DialogHeader>

        <div className="grid min-h-0 flex-1 gap-3 overflow-y-auto px-5 py-4 sm:grid-cols-2">
          <Field label="天数">
            <select
              aria-label="天数"
              className="h-10 w-full rounded-md border border-input bg-card px-3 text-sm"
              value={options.days}
              onChange={(event) => onChange({ days: event.target.value })}
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
              value={options.pace}
              onChange={(event) => onChange({ pace: event.target.value })}
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
              value={options.partySize}
              onChange={(event) => onChange({ partySize: event.target.value })}
            />
          </Field>
          <Field label="总预算（元）">
            <Input
              aria-label="总预算"
              type="number"
              min={0}
              step={100}
              value={options.budgetYuan}
              onChange={(event) => onChange({ budgetYuan: event.target.value })}
            />
          </Field>
          <Field label="每日游玩上限（小时）">
            <Input
              aria-label="每日游玩上限"
              type="number"
              min={1}
              max={16}
              value={options.maxHours}
              onChange={(event) => onChange({ maxHours: event.target.value })}
            />
          </Field>
          <Field label="感兴趣">
            <Input
              aria-label="感兴趣"
              placeholder="历史、咖啡、夜景"
              value={options.interests}
              onChange={(event) => onChange({ interests: event.target.value })}
            />
          </Field>
          <Field label="必须去">
            <Input
              aria-label="必须去"
              placeholder="故宫、天坛"
              value={options.mustVisit}
              onChange={(event) => onChange({ mustVisit: event.target.value })}
            />
          </Field>
          <Field label="避开">
            <Input
              aria-label="避开"
              placeholder="人多、爬山、辣"
              value={options.avoid}
              onChange={(event) => onChange({ avoid: event.target.value })}
            />
          </Field>
          <label className="flex items-center gap-2 text-sm sm:col-span-2">
            <input
              type="checkbox"
              checked={options.meals}
              onChange={(event) => onChange({ meals: event.target.checked })}
            />
            安排午餐和晚餐
          </label>
        </div>

        <div className="flex items-center justify-between gap-3 border-t border-hairline px-5 py-3">
          <Button type="button" size="sm" variant="ghost" onClick={onReset} disabled={count === 0}>
            清空偏好
          </Button>
          <Button type="button" size="sm" onClick={() => onOpenChange(false)}>
            完成
          </Button>
        </div>
      </DialogContent>
    </Dialog>
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
