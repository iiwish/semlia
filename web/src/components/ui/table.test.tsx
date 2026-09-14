import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { Table, TableBody, TableCell, TableOpenButton, TableRow } from "./table";

it("opens a record by row or keyboard without swallowing embedded commands", async () => {
  const user = userEvent.setup();
  const open = vi.fn();
  const edit = vi.fn();
  render(<Table><TableBody><TableRow onOpen={open}><TableCell><TableOpenButton onClick={open}>订单来源</TableOpenButton></TableCell><TableCell>已连接</TableCell><TableCell><button onClick={edit}>编辑</button><button disabled>删除</button></TableCell></TableRow></TableBody></Table>);
  await user.click(screen.getByText("已连接"));
  expect(open).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: "订单来源" })).toHaveFocus();
  await user.keyboard("{Enter}");
  expect(open).toHaveBeenCalledTimes(2);
  await user.click(screen.getByRole("button", { name: "编辑" }));
  await user.click(screen.getByRole("button", { name: "删除" }));
  expect(edit).toHaveBeenCalledTimes(1);
  expect(open).toHaveBeenCalledTimes(2);
});

it("does not turn text selection into navigation", async () => {
  const user = userEvent.setup();
  const open = vi.fn();
  const selection = vi.spyOn(window, "getSelection").mockReturnValue({ toString: () => "source-id" } as Selection);
  render(<Table><TableBody><TableRow onOpen={open}><TableCell>source-id</TableCell></TableRow></TableBody></Table>);
  try {
    await user.click(screen.getByText("source-id"));
    expect(open).not.toHaveBeenCalled();
  } finally { selection.mockRestore(); }
});
