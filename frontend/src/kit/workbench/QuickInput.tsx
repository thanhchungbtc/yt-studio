import { useEffect, useRef, useState } from "react";
import { Dialog as RDialog } from "radix-ui";
import { CornerDownLeft } from "lucide-react";
import { create } from "zustand";
import { cn } from "@/kit/lib/cn";
import { Spinner } from "@/kit/ui/misc";
import { afterMenus } from "@/kit/lib/focus";

interface AskTextOptions {
  title: string;
  placeholder?: string;
  value?: string;
  hint?: string;
  allowEmpty?: boolean;
  validate?: (value: string) => string | undefined | Promise<string | undefined>;
}

interface Request extends AskTextOptions {
  resolve: (value: string | undefined) => void;
}

const useRequest = create<{ request?: Request }>(() => ({}));

let pending: Request | undefined;

export function askText(opts: AskTextOptions): Promise<string | undefined> {
  return new Promise((resolve) => {
    useRequest.getState().request?.resolve(undefined);
    pending?.resolve(undefined);
    const request = { ...opts, resolve };
    pending = request;
    afterMenus(() => {
      if (pending !== request) return;
      pending = undefined;
      useRequest.setState({ request });
    });
  });
}

export function QuickInputHost() {
  const request = useRequest((s) => s.request);
  const close = (value: string | undefined) => {
    request?.resolve(value);
    useRequest.setState({ request: undefined });
  };
  return (
    <RDialog.Root open={!!request} onOpenChange={(o) => !o && close(undefined)}>
      <RDialog.Portal>
        <RDialog.Content
          aria-describedby={undefined}
          className="glass-pop glass-dense fixed top-[12vh] left-1/2 z-50 w-[min(520px,calc(100vw-48px))] -translate-x-1/2 animate-pop-in overflow-hidden rounded-[18px] outline-none"
        >
          {request && <Prompt key={request.title} request={request} onDone={close} />}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}

function Prompt({ request, onDone }: { request: Request; onDone: (value: string | undefined) => void }) {
  const [value, setValue] = useState(request.value ?? "");
  const [problem, setProblem] = useState<string>();
  const [checking, setChecking] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const seq = useRef(0);

  useEffect(() => {
    input.current?.select();
  }, []);

  useEffect(() => {
    if (!request.validate) return;
    const n = ++seq.current;
    if (!value.trim()) {
      setProblem(undefined);
      setChecking(false);
      return;
    }
    const t = setTimeout(async () => {
      setChecking(true);
      const why = await request.validate!(value);
      if (n !== seq.current) return;
      setChecking(false);
      setProblem(why);
    }, 120);
    return () => clearTimeout(t);
  }, [value]); // eslint-disable-line react-hooks/exhaustive-deps

  const submit = async () => {
    if (!value.trim() && !request.allowEmpty) return;
    if (request.validate && value.trim()) {
      const why = await request.validate(value);
      if (why) return setProblem(why);
    }
    onDone(value);
  };

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      <RDialog.Title className="px-4 pt-3 text-2xs font-semibold tracking-wide text-fg-subtle uppercase">{request.title}</RDialog.Title>
      <div className="flex items-center gap-2 px-4">
        <input
          ref={input}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder={request.placeholder}
          spellCheck={false}
          autoComplete="off"
          aria-invalid={!!problem}
          className="h-11 min-w-0 flex-1 bg-transparent text-md text-fg outline-none placeholder:text-fg-subtle"
        />
        {checking ? <Spinner className="size-3.5 text-fg-subtle" /> : <CornerDownLeft className={cn("size-3.5 text-fg-subtle transition-opacity", (problem || (!value.trim() && !request.allowEmpty)) && "opacity-30")} />}
      </div>
      <div role={problem ? "alert" : undefined} className={cn("border-t border-line px-4 py-2 text-xs", problem ? "text-danger" : "text-fg-subtle")}>
        {problem ?? request.hint ?? "Press ↵ to confirm, Esc to cancel"}
      </div>
    </form>
  );
}
