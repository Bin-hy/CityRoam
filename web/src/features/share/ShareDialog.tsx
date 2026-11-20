import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Check, Copy, Share2 } from "lucide-react";
import { ApiError } from "@/api/client";
import { createShare } from "@/api/trips";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";

export function ShareDialog({ tripID }: { tripID: string }) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const share = useMutation({ mutationFn: () => createShare(tripID) });

  const link = share.data ? new URL(share.data.url, window.location.origin).toString() : "";

  function changeOpen(next: boolean) {
    setOpen(next);
    if (next && !share.data && !share.isPending) {
      setCopied(false);
      share.mutate();
    }
  }

  async function copyLink() {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  const error =
    share.error instanceof ApiError
      ? share.error.message
      : share.error instanceof Error
        ? share.error.message
        : null;

  return (
    <>
      <Button size="sm" variant="outline" onClick={() => changeOpen(true)}>
        <Share2 className="mr-1 h-4 w-4" />
        分享
      </Button>
      <Dialog open={open} onOpenChange={changeOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>分享行程</DialogTitle>
            <DialogDescription>生成无需登录的只读链接，只展示当前最终版本。</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            {share.isPending && (
              <p role="status" className="text-sm text-muted-foreground">
                正在生成链接…
              </p>
            )}
            {error && <Alert variant="destructive">{error}</Alert>}
            {share.data && (
              <>
                <Input
                  aria-label="分享链接"
                  readOnly
                  value={link}
                  className="border-hairline bg-paper font-mono text-xs"
                  onFocus={(event) => event.target.select()}
                />
                <div className="flex flex-wrap items-center gap-2">
                  <Button size="sm" onClick={() => void copyLink()}>
                    {copied ? <Check className="mr-1 h-4 w-4" /> : <Copy className="mr-1 h-4 w-4" />}
                    {copied ? "已复制" : "复制链接"}
                  </Button>
                  <Button size="sm" variant="outline" asChild>
                    <a href={link} target="_blank" rel="noreferrer">
                      打开分享页
                    </a>
                  </Button>
                </div>
                <p className="border-t border-hairline pt-3 text-xs leading-relaxed text-muted-foreground">
                  分享页含 AI 生成标识与来源；链接可转发，但不包含你的账号信息。
                </p>
              </>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
