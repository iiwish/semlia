import type { ComponentProps } from "react";
import { ChevronRight } from "lucide-react";

type TableProps = ComponentProps<"table">;
function mergeClassNames(...names: Array<string | undefined>) {
  return names.filter(Boolean).join(" ");
}

export function TableViewport({ className, ...props }: ComponentProps<"div">) {
  return <div className={mergeClassNames("table-viewport", className)} {...props} />;
}

export function TablePanel({ className, ...props }: ComponentProps<"section">) {
  return <section className={mergeClassNames("source-table", className)} {...props} />;
}

export function TableFooter({ className, ...props }: ComponentProps<"footer">) {
  return <footer className={mergeClassNames("source-table-footer", className)} {...props} />;
}

export function Table({ className, ...props }: TableProps) {
  return <table className={mergeClassNames("data-table", className)} {...props} />;
}

export function TableHeader({ className, ...props }: ComponentProps<"thead">) {
  return <thead className={mergeClassNames("data-table-header", className)} {...props} />;
}

export function TableBody({ className, ...props }: ComponentProps<"tbody">) {
  return <tbody className={mergeClassNames("data-table-body", className)} {...props} />;
}

export function TableRow({ className, onOpen, onClick, ...props }: ComponentProps<"tr"> & { onOpen?: () => void }) {
  return <tr className={mergeClassNames("data-table-row", onOpen ? "data-table-row-openable" : undefined, className)} {...props} onClick={(event) => {
    onClick?.(event);
    if (event.defaultPrevented || !onOpen) return;
    // Embedded commands and text selection must not open the row detail.
    if ((event.target as Element).closest("button, a, input, select, textarea, [role=button]")) return;
    if (window.getSelection()?.toString()) return;
    event.currentTarget.querySelector<HTMLButtonElement>(".data-table-open")?.focus({ preventScroll: true });
    onOpen();
  }} />;
}

export function TableOpenButton({ className, children, ...props }: ComponentProps<"button">) {
  return <button type="button" className={mergeClassNames("data-table-open", className)} {...props}>{children}<ChevronRight size={14} aria-hidden="true" /></button>;
}

export function TableHead({ className, ...props }: ComponentProps<"th">) {
  return <th className={mergeClassNames("data-table-head", className)} {...props} />;
}

export function TableCell({ className, ...props }: ComponentProps<"td">) {
  return <td className={mergeClassNames("data-table-cell", className)} {...props} />;
}
