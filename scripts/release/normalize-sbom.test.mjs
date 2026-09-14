import assert from "node:assert/strict";
import test from "node:test";
import { normalizeSBOM } from "./normalize-sbom.mjs";

test("preserves vulnerability evidence and only removes the absent-method sentinel", () => {
  const document = {
    components: [{ "bom-ref": "example" }],
    dependencies: [{ ref: "example", dependsOn: [] }],
    vulnerabilities: [{ id: "example", affects: [{ ref: "example" }], ratings: [
      { source: { name: "vendor" }, severity: "high", method: "Null" },
      { severity: "high", method: "CVSSv31", score: 7.5, vector: "vector" },
    ] }],
  };
  const expected = structuredClone(document);
  delete expected.vulnerabilities[0].ratings[0].method;
  assert.deepEqual(normalizeSBOM(document), expected);
});

test("refuses to normalize a scored rating with an unknown method", () => {
  assert.throws(() => normalizeSBOM({ vulnerabilities: [{ ratings: [{ method: "Null", score: 7.5 }] }] }));
  assert.deepEqual(normalizeSBOM({ components: [] }), { components: [] });
});
