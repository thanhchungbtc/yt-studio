import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import { create } from "zustand";
import { ContextMenu as RContext, DropdownMenu as RMenu } from "radix-ui";
import type { LucideIcon } from "lucide-react";
import { Check, ChevronRight } from "lucide-react";
import { cn } from "@/kit/lib/cn";
import { keepFocusIfMoved } from "@/kit/lib/focus";
import { Kbd } from "./Kbd";

/**
 * Menu content is described as data so the same items render in dropdowns
 * and context menus.
 */
export type MenuEntry =
  | {
      type?: "item";
      label: string;
      icon?: LucideIcon;
      /** A second, muted line explaining the item. */
      description?: string;
      /** Shown at the right edge, before any shortcut. */
      detail?: ReactNode;
      shortcut?: string;
      danger?: boolean;
      disabled?: boolean;
      checked?: boolean;
      /** A CSS color shown as a dot in place of the icon (e.g. palette colors). */
      swatch?: string;
      onSelect: () => void;
    }
  | { type: "separator" }
  | { type: "label"; label: string }
  | { type: "submenu"; label: string; icon?: LucideIcon; detail?: ReactNode; items: MenuEntry[] };

const contentClass =
  "glass-pop z-50 min-w-48 animate-pop-in overflow-hidden rounded-[12px] p-1 text-sm text-fg outline-none";
const itemClass =
  "relative flex h-7 cursor-default items-center gap-2 rounded-[8px] px-2 text-fg outline-none select-none data-[disabled]:opacity-40 data-[highlighted]:bg-row-highlight";

type Parts = typeof RMenu | typeof RContext;

function renderEntries(P: Parts, entries: MenuEntry[]): ReactNode {
  return entries.map((e, i) => {
    if (e.type === "separator") return <P.Separator key={i} className="mx-1 my-1 h-px bg-line" />;
    if (e.type === "label")
      return (
        <P.Label key={i} className="px-2 pt-1.5 pb-1 text-2xs font-semibold tracking-wide text-fg-subtle uppercase">
          {e.label}
        </P.Label>
      );
    if (e.type === "submenu") {
      const Icon = e.icon;
      return (
        <P.Sub key={i}>
          <P.SubTrigger className={itemClass}>
            {Icon ? <Icon className="size-3.5 opacity-80" /> : <span className="size-3.5" />}
            <span className="flex-1">{e.label}</span>
            {e.detail && <span className="flex items-center gap-1.5 text-xs text-fg-subtle">{e.detail}</span>}
            <ChevronRight className="size-3.5 opacity-60" />
          </P.SubTrigger>
          <P.Portal>
            <P.SubContent className={contentClass} sideOffset={4}>
              {renderEntries(P, e.items)}
            </P.SubContent>
          </P.Portal>
        </P.Sub>
      );
    }
    const Icon = e.icon;
    // With a description the row grows; the lead stays centered on the first line.
    const lead = cn("size-3.5 shrink-0", e.description && "mt-px");
    return (
      <P.Item
        key={i}
        disabled={e.disabled}
        onSelect={e.onSelect}
        className={cn(
          itemClass,
          e.description && "h-auto items-start py-1.5",
          e.danger && "text-danger data-[highlighted]:bg-[color-mix(in_srgb,var(--danger)_12%,transparent)]",
        )}
      >
        {e.swatch ? (
          <span className={cn(lead, "flex items-center justify-center")}>
            <span className="size-2.5 rounded-full" style={{ background: e.swatch }} />
          </span>
        ) : e.checked !== undefined ? (
          <Check className={cn(lead, !e.checked && "invisible")} />
        ) : Icon ? (
          <Icon className={cn(lead, "opacity-80")} />
        ) : (
          <span className={lead} />
        )}
        {e.description ? (
          <span className="flex min-w-0 flex-auto flex-col gap-0.5">
            <span className="truncate leading-4">{e.label}</span>
            <span className="max-w-72 text-xs leading-4 text-fg-subtle">{e.description}</span>
          </span>
        ) : (
          <span className="flex-1 truncate">{e.label}</span>
        )}
        {e.detail && <span className="flex h-4 items-center text-fg-subtle">{e.detail}</span>}
        {e.swatch && e.checked && <Check className="size-3.5 text-fg-muted" />}
        {e.shortcut && <Kbd keys={e.shortcut} className="opacity-70" />}
      </P.Item>
    );
  });
}

export function DropdownMenu({
  trigger,
  items,
  align = "start",
  side = "bottom",
}: {
  trigger: ReactNode;
  items: MenuEntry[];
  align?: "start" | "center" | "end";
  side?: "top" | "bottom";
}) {
  return (
    <RMenu.Root>
      <RMenu.Trigger asChild>{trigger}</RMenu.Trigger>
      <RMenu.Portal>
        <RMenu.Content align={align} side={side} sideOffset={5} collisionPadding={8} className={contentClass} onCloseAutoFocus={keepFocusIfMoved}>
          <Entries parts={RMenu} items={items} />
        </RMenu.Content>
      </RMenu.Portal>
    </RMenu.Root>
  );
}

/**
 * Items rendered only when the menu opens (closed menus build nothing; a
 * function sees the state of that moment).
 */
function Entries({ parts = RContext, items }: { parts?: Parts; items: MenuEntry[] | (() => MenuEntry[]) }) {
  return <>{renderEntries(parts, typeof items === "function" ? items() : items)}</>;
}

export function ContextMenu({
  children,
  items,
  onOpenChange,
}: {
  children: ReactNode;
  items: MenuEntry[] | (() => MenuEntry[]);
  onOpenChange?: (open: boolean) => void;
}) {
  return (
    <RContext.Root onOpenChange={onOpenChange}>
      <RContext.Trigger asChild>{children}</RContext.Trigger>
      <RContext.Portal>
        <RContext.Content collisionPadding={8} className={contentClass} onCloseAutoFocus={keepFocusIfMoved}>
          <Entries items={items} />
        </RContext.Content>
      </RContext.Portal>
    </RContext.Root>
  );
}

const useMenuAt = create<{ menu?: { x: number; y: number; items: MenuEntry[]; key: number } }>(() => ({}));

/**
 * Opens a context menu at a point (e.g. where a link was right-clicked):
 * one menu for many small targets, which then needn't each have their own.
 */
export function openMenuAt(x: number, y: number, items: MenuEntry[]) {
  useMenuAt.setState({ menu: { x, y, items, key: Date.now() } });
}

export function MenuAtHost() {
  const menu = useMenuAt((s) => s.menu);
  if (!menu) return null;
  return (
    <RMenu.Root key={menu.key} open onOpenChange={(open) => !open && useMenuAt.setState({ menu: undefined })}>
      {/* The menu's anchor: a point, outside the zoomed app root. */}
      {createPortal(
        <RMenu.Trigger asChild>
          <span aria-hidden className="pointer-events-none fixed size-0" style={{ left: menu.x, top: menu.y }} />
        </RMenu.Trigger>,
        document.body,
      )}
      <RMenu.Portal>
        <RMenu.Content align="start" side="bottom" sideOffset={2} collisionPadding={8} className={contentClass} onCloseAutoFocus={(e) => e.preventDefault()}>
          {renderEntries(RMenu, menu.items)}
        </RMenu.Content>
      </RMenu.Portal>
    </RMenu.Root>
  );
}
