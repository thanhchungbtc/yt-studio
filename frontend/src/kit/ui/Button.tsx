import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/kit/lib/cn";
import { Tooltip } from "./Tooltip";

type Variant = "primary" | "secondary" | "ghost" | "danger" | "subtle";
type Size = "xs" | "sm" | "md";

const variants: Record<Variant, string> = {
  primary: "bg-accent-fill text-accent-fg hover:bg-accent-fill-hover shadow-[inset_0_1px_0_rgb(255_255_255/0.18),0_1px_2px_rgb(0_0_0/0.12)]",
  secondary: "bg-content text-fg hairline hover:bg-hover shadow-card",
  ghost: "text-fg-muted hover:text-fg hover:bg-hover",
  subtle: "bg-well text-fg hover:bg-active",
  danger: "bg-danger-fill text-white hover:brightness-110",
};

const sizes: Record<Size, string> = {
  xs: "h-6 px-2.5 text-xs gap-1 rounded-full",
  sm: "h-7 px-3 text-sm gap-1.5 rounded-full",
  md: "h-8 px-4 text-base gap-2 rounded-full",
};

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  icon?: LucideIcon;
  trailing?: ReactNode;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "secondary", size = "sm", icon: Icon, trailing, className, children, type = "button", ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={cn(
        "inline-flex shrink-0 items-center justify-center font-medium whitespace-nowrap transition-[background-color,color,filter] duration-100 select-none disabled:pointer-events-none disabled:opacity-45",
        variants[variant],
        sizes[size],
        className,
      )}
      {...rest}
    >
      {Icon && <Icon className={size === "md" ? "size-4" : "size-3.5"} strokeWidth={2} />}
      {children}
      {trailing}
    </button>
  );
});

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  icon: LucideIcon;
  label: string;
  /** Command id whose keybinding is shown in the tooltip. */
  command?: string;
  size?: "xs" | "sm" | "md";
  active?: boolean;
  tooltipSide?: "top" | "bottom" | "left" | "right";
}

const iconSizes = { xs: "size-6", sm: "size-7", md: "size-8" };

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { icon: Icon, label, command, size = "sm", active, className, tooltipSide = "bottom", type = "button", ...rest },
  ref,
) {
  return (
    <Tooltip content={label} command={command} side={tooltipSide}>
      <button
        ref={ref}
        type={type}
        aria-label={label}
        aria-pressed={active}
        className={cn(
          "inline-flex shrink-0 items-center justify-center rounded-full text-fg-muted transition-colors duration-100 hover:bg-hover hover:text-fg disabled:pointer-events-none disabled:opacity-40",
          active && "bg-active text-fg",
          iconSizes[size],
          className,
        )}
        {...rest}
      >
        <Icon className={size === "xs" ? "size-3.5" : "size-4"} strokeWidth={1.8} />
      </button>
    </Tooltip>
  );
});
