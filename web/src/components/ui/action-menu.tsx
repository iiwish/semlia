import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Ellipsis } from "lucide-react";

export interface MenuAction {
  label: string;
  icon: ReactNode;
  onSelect: () => void;
  disabled?: boolean;
  reason?: string;
  danger?: boolean;
}

export function ActionMenu({ label, actions }: { label: string; actions: MenuAction[] }) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState<{ top: number; left: number } | null>(null);
  const close = () => { setPosition(null); trigger.current?.focus(); };
  useEffect(() => {
    if (!position) return;
    (menu.current?.querySelector<HTMLButtonElement>("button:not(:disabled)") ?? menu.current)?.focus();
    const dismiss = (event: Event) => {
      if (event.target instanceof Node && (menu.current?.contains(event.target) || trigger.current?.contains(event.target))) return;
      setPosition(null);
    };
    document.addEventListener("pointerdown", dismiss);
    window.addEventListener("resize", dismiss);
    document.addEventListener("scroll", dismiss, true);
    return () => { document.removeEventListener("pointerdown", dismiss); window.removeEventListener("resize", dismiss); document.removeEventListener("scroll", dismiss, true); };
  }, [position]);
  return <>
    <button ref={trigger} className="icon-button" type="button" title="更多操作" aria-label={label} aria-haspopup="menu" aria-expanded={Boolean(position)} aria-controls={position ? id : undefined} onClick={() => {
      if (position) { close(); return; }
      const rect = trigger.current!.getBoundingClientRect();
      const height = actions.length * 42 + 12;
      setPosition({ left: Math.max(8, Math.min(rect.right - 190, window.innerWidth - 198)), top: rect.bottom + height + 8 > window.innerHeight ? Math.max(8, rect.top - height - 4) : rect.bottom + 4 });
    }}><Ellipsis size={16} /></button>
    {position && createPortal(<div ref={menu} tabIndex={-1} id={id} className="table-action-menu" role="menu" aria-label={label} style={position} onClick={(event) => event.stopPropagation()} onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node)) setPosition(null); }} onKeyDown={(event) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(); }
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const buttons = [...menu.current!.querySelectorAll<HTMLButtonElement>("button:not(:disabled)")];
      const current = buttons.indexOf(document.activeElement as HTMLButtonElement);
      const next = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1 : (current + (event.key === "ArrowDown" ? 1 : -1) + buttons.length) % buttons.length;
      buttons[next]?.focus();
    }}>{actions.map((action) => <button key={action.label} type="button" role="menuitem" disabled={action.disabled} title={action.reason} className={action.danger ? "is-danger" : undefined} onClick={() => { close(); action.onSelect(); }}>{action.icon}<span>{action.label}</span></button>)}</div>, document.body)}
  </>;
}
