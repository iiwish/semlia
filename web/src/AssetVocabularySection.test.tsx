import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { AssetVocabularySection } from "./AssetVocabularySection";
import { emptyVocabulary } from "./knowledge";
import { assets } from "./testing/data";

it("shows governed wording and opens the vocabulary revision", async () => {
  const onStartRevision = vi.fn();
  const asset = { ...assets[0], knowledgeSpec: { members: [{ id: "region", name: "当前区域", aliases: ["大区"], valueAliases: [{ value: "华东", aliases: ["江浙沪", "华东地区"] }] }] }, vocabulary: { ...emptyVocabulary(), aliases: ["GMV", "成交额"], excludes: ["无效订单"], confusableWith: [{ ref: { assetId: "ast_booked", revisionId: "rev_b", releaseId: "rls_b" }, distinction: "下单金额含无效订单" }], sampleQuestions: [{ question: "8月 GMV 是多少？" }] } };
  render(<AssetVocabularySection asset={asset} onStartRevision={onStartRevision} />);
  const section = screen.getByRole("region", { name: "业务词汇" });
  expect(section).toHaveTextContent("别名GMV；成交额");
  expect(section).toHaveTextContent("明确排除无效订单");
  expect(section).toHaveTextContent("易混淆知识已发布知识：下单金额含无效订单");
  expect(section).toHaveTextContent("典型问题8月 GMV 是多少？");
  expect(section).toHaveTextContent("属性与取值别名当前区域（大区）；华东 ← 江浙沪、华东地区");
  await userEvent.click(screen.getByRole("button", { name: "修订业务词汇" }));
  expect(onStartRevision).toHaveBeenCalledWith({ assetId: asset.id, fieldPath: "vocabulary", origin: "definition" });
});

it("says what is missing when nothing is registered", () => {
  render(<AssetVocabularySection asset={{ ...assets[0], aliases: [], includes: [], excludes: [], examples: [], vocabulary: emptyVocabulary() }} onStartRevision={vi.fn()} />);
  expect(screen.getByText(/尚未登记别名、易混淆知识或典型问题/)).toBeVisible();
});
