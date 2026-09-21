import { describe, expect, it } from "vitest";
import { findingMessage, validatorLabel } from "./knowledgeMessages";
import { ProductionApiError, productionMessage } from "./semanticProduction";

describe("knowledge workflow messages", () => {
  it("turns missing business knowledge into actions", () => {
    expect(findingMessage("PRODUCTION_DEFINITION_UNRESOLVED")).toContain("补充业务定义");
    expect(findingMessage("PRODUCTION_SCOPE_UNRESOLVED")).toContain("适用范围");
    expect(findingMessage("PRODUCTION_BUSINESS_RULE_UNCONFIRMED")).toContain("确认业务口径");
    expect(validatorLabel("schema")).toBe("知识内容检查");
  });
  it("does not expose unknown diagnostics as user instructions", () => {
    expect(findingMessage("UNKNOWN_INTERNAL_CODE", "blocker")).toBe("检查未通过，请展开技术详情并联系管理员。");
  });
  it("explains unavailable production without printing the server message", () => {
    const message = productionMessage(new ProductionApiError("the production dependency is unavailable", "DEPENDENCY_UNAVAILABLE", 503, "test-trace"));
    expect(message).toContain("知识整理服务尚未就绪");
    expect(message).toContain("test-trace");
    expect(message).not.toContain("the production");
  });
});
