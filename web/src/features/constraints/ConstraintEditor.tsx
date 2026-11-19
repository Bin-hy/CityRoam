import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export type Hardness = "hard" | "soft";

export interface EditableConstraint {
  key: string;
  label: string;
  value: string;
  hardness: Hardness;
}

interface ConstraintEditorProps {
  items: EditableConstraint[];
  onChange: (items: EditableConstraint[]) => void;
}

const groups: { hardness: Hardness; title: string; hint: string }[] = [
  { hardness: "hard", title: "硬约束", hint: "必须满足，违反时给出修复或提示" },
  { hardness: "soft", title: "软约束", hint: "尽量满足，可被权衡取舍" },
];

export function ConstraintEditor({ items, onChange }: ConstraintEditorProps) {
  const toggle = (key: string): EditableConstraint[] =>
    items.map((item) =>
      item.key === key ? { ...item, hardness: (item.hardness === "hard" ? "soft" : "hard") as Hardness } : item,
    );

  const edit = (key: string, value: string) =>
    items.map((item) => (item.key === key ? { ...item, value } : item));

  return (
    <div className="grid gap-4 md:grid-cols-2">
      {groups.map((group) => {
        const groupItems = items.filter((item) => item.hardness === group.hardness);
        return (
          <section key={group.hardness} className="rounded-lg border bg-card p-4">
            <header className="mb-3">
              <h3 className="text-sm font-semibold">{group.title}</h3>
              <p className="text-xs text-muted-foreground">{group.hint}</p>
            </header>
            {groupItems.length === 0 ? (
              <p className="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground">暂无{group.title}</p>
            ) : (
              <ul className="space-y-2">
                {groupItems.map((item) => (
                  <li key={item.key} className="flex items-center gap-2">
                    <span className={cn("w-20 shrink-0 text-sm", group.hardness === "hard" && "font-medium")}>
                      {item.label}
                    </span>
                    <Input
                      aria-label={`${item.label}的值`}
                      value={item.value}
                      onChange={(event) => onChange(edit(item.key, event.target.value))}
                      className="h-8"
                    />
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => onChange(toggle(item.key))}
                      aria-label={`将${item.label}改为${group.hardness === "hard" ? "软约束" : "硬约束"}`}
                    >
                      {group.hardness === "hard" ? "转软" : "转硬"}
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </section>
        );
      })}
    </div>
  );
}
