import type { ReactNode } from "react";
import { create } from "zustand";
import { Dialog } from "@/kit/ui/Dialog";
import { Button } from "@/kit/ui/Button";

/** A button of a choice dialog. */
export interface Choice<T extends string> {
  label: string;
  value: T;
  variant?: "primary" | "danger" | "ghost" | "secondary";
  /** Placed on the left, apart from the others (macOS: "Don't Save"). */
  apart?: boolean;
  /** ⌘ plus this key picks it (macOS: ⌘D for "Don't Save"). */
  key?: string;
}

interface Request {
  title: string;
  message?: ReactNode;
  choices: Choice<string>[];
  cancel: string;
  resolve: (value: string) => void;
}

const useRequest = create<{ request?: Request }>(() => ({}));

/**
 * Asks the user to pick one of several actions. Resolves with the chosen
 * value, or `cancel` when dismissed (Escape, clicking outside).
 */
export function choose<T extends string>(opts: { title: string; message?: ReactNode; choices: Choice<T>[]; cancel: T }): Promise<T> {
  return new Promise((resolve) => {
    // A newer question replaces (cancels) an unanswered one.
    useRequest.getState().request?.resolve(useRequest.getState().request!.cancel);
    useRequest.setState({ request: { ...opts, resolve: resolve as (v: string) => void } });
  });
}

interface ConfirmOptions {
  title: string;
  message?: string;
  confirmLabel?: string;
  danger?: boolean;
}

/** Asks the user to confirm an action. Resolves true when confirmed. */
export async function confirm(opts: ConfirmOptions): Promise<boolean> {
  const answer = await choose({
    title: opts.title,
    message: opts.message,
    cancel: "cancel",
    choices: [
      { label: "Cancel", value: "cancel", variant: "ghost" },
      { label: opts.confirmLabel ?? "Confirm", value: "ok", variant: opts.danger ? "danger" : "primary" },
    ],
  });
  return answer === "ok";
}

export function ConfirmHost() {
  const request = useRequest((s) => s.request);
  const close = (value: string) => {
    request?.resolve(value);
    useRequest.setState({ request: undefined });
  };
  const apart = request?.choices.filter((c) => c.apart) ?? [];
  const rest = request?.choices.filter((c) => !c.apart) ?? [];
  const primary = request?.choices.find((c) => c.variant === "primary" || c.variant === "danger");
  // A destructive action is never the default: Return picks the safe choice.
  const focused = primary?.variant === "danger" ? request?.choices.find((c) => c.value === request.cancel) : primary;
  return (
    <Dialog open={!!request} onOpenChange={(o) => !o && request && close(request.cancel)} title={request?.title} description={request?.message} className="w-[440px]">
      <div
        className="flex items-center gap-2 px-5 pt-2 pb-4"
        onKeyDown={(e) => {
          const hit = e.metaKey && request?.choices.find((c) => c.key && c.key === e.key.toLowerCase());
          if (hit) {
            e.preventDefault();
            close(hit.value);
          }
        }}
      >
        {apart.map((c) => (
          <Button key={c.value} variant={c.variant ?? "secondary"} autoFocus={c === focused} onClick={() => close(c.value)}>
            {c.label}
          </Button>
        ))}
        <div className="flex-1" />
        {rest.map((c) => (
          <Button key={c.value} variant={c.variant ?? "secondary"} autoFocus={c === focused} onClick={() => close(c.value)}>
            {c.label}
          </Button>
        ))}
      </div>
    </Dialog>
  );
}
