import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Switch as RSwitch } from "radix-ui";
import { cn } from "@/kit/lib/cn";
import { SlidingThumb } from "./SlidingThumb";

export function Spinner({ className }: { className?: string }) {
  return (
    <svg className={cn("size-3.5 animate-spin text-current", className)} viewBox="0 0 16 16" fill="none" aria-hidden>
      <circle cx="8" cy="8" r="6.5" stroke="currentColor" strokeOpacity="0.2" strokeWidth="2" />
      <path d="M14.5 8A6.5 6.5 0 0 0 8 1.5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

/** Three softly pulsing dots, used while the model is thinking. */
export function PulseDots({ className }: { className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-[3px]", className)} aria-hidden>
      {[0, 1, 2].map((i) => (
        <span
          key={i}
          className="size-1 rounded-full bg-current opacity-40"
          style={{ animation: `pulse-dot 1.2s ease-in-out ${i * 0.16}s infinite` }}
        />
      ))}
    </span>
  );
}

type Tone = "neutral" | "accent" | "success" | "warning" | "danger" | "info";
const tones: Record<Tone, string> = {
  neutral: "bg-well text-fg-muted",
  accent: "bg-accent-soft text-accent",
  success: "bg-[color-mix(in_srgb,var(--success)_14%,transparent)] text-success",
  warning: "bg-[color-mix(in_srgb,var(--warning)_16%,transparent)] text-warning",
  danger: "bg-[color-mix(in_srgb,var(--danger)_14%,transparent)] text-danger",
  info: "bg-[color-mix(in_srgb,var(--info)_14%,transparent)] text-info",
};

export function Badge({ tone = "neutral", children, className }: { tone?: Tone; children: ReactNode; className?: string }) {
  return (
    <span className={cn("inline-flex h-[18px] shrink-0 items-center gap-1 rounded-full px-1.5 text-2xs font-medium", tones[tone], className)}>
      {children}
    </span>
  );
}

export function Switch({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label?: string }) {
  return (
    <RSwitch.Root
      checked={checked}
      onCheckedChange={onChange}
      aria-label={label}
      className="relative h-[18px] w-[30px] shrink-0 rounded-full bg-active transition-colors data-[state=checked]:bg-accent-fill"
    >
      <RSwitch.Thumb className="block size-[14px] translate-x-[2px] rounded-full bg-white shadow-sm transition-transform duration-150 data-[state=checked]:translate-x-[14px]" />
    </RSwitch.Root>
  );
}

export function EmptyState({
  icon: Icon,
  title,
  children,
  action,
  className,
}: {
  icon?: LucideIcon;
  title: string;
  children?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col items-center justify-center gap-2 px-6 py-10 text-center", className)}>
      {Icon && <Icon className="mb-1 size-7 text-fg-faint" strokeWidth={1.5} />}
      <div className="text-sm font-medium text-fg-muted">{title}</div>
      {children && <div className="max-w-64 text-xs text-fg-subtle">{children}</div>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}

export function SectionHeader({ children, actions, className }: { children: ReactNode; actions?: ReactNode; className?: string }) {
  return (
    <div className={cn("flex h-7 items-center gap-1 px-3 text-2xs font-semibold tracking-wide text-fg-subtle uppercase", className)}>
      <span className="flex-1 truncate">{children}</span>
      {actions}
    </div>
  );
}

export function Segmented<T extends string>({
  value,
  options,
  onChange,
  className,
}: {
  value: T;
  options: Array<{ value: T; label: string }>;
  onChange: (v: T) => void;
  className?: string;
}) {
  return (
    <div className={cn("relative inline-flex rounded-full bg-well p-0.5", className)} role="radiogroup">
      <SlidingThumb active={value} />
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          data-thumb-key={o.value}
          aria-checked={o.value === value}
          onClick={() => onChange(o.value)}
          className={cn(
            "relative h-6 rounded-full px-2.5 text-xs font-medium text-fg-muted transition-colors duration-150",
            o.value === value ? "text-fg" : "hover:text-fg",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function TextInput({
  value,
  onChange,
  placeholder,
  className,
  autoFocus,
  onKeyDown,
  icon: Icon,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  className?: string;
  autoFocus?: boolean;
  onKeyDown?: (e: React.KeyboardEvent<HTMLInputElement>) => void;
  icon?: LucideIcon;
}) {
  return (
    <label className={cn("flex h-7 items-center gap-1.5 rounded-full bg-well px-2.5 hairline focus-within:shadow-[0_0_0_2px_var(--focus)]", className)}>
      {Icon && <Icon className="size-3.5 shrink-0 text-fg-subtle" />}
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoFocus={autoFocus}
        onKeyDown={onKeyDown}
        spellCheck={false}
        className="min-w-0 flex-1 bg-transparent text-sm text-fg outline-none placeholder:text-fg-subtle"
      />
    </label>
  );
}
