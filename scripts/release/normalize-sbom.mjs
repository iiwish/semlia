import { readFileSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

export function normalizeSBOM(document) {
  for (const vulnerability of document.vulnerabilities ?? []) {
    for (const rating of vulnerability.ratings ?? []) {
      if (rating.method !== "Null") continue;
      if (rating.score != null || rating.vector != null) {
        throw new Error("Refusing to remove a scoring method from a scored rating");
      }
      // CycloneDX CLI emits its internal sentinel for an absent optional method.
      delete rating.method;
    }
  }
  return document;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const path = process.argv[2];
  if (!path) throw new Error("SBOM path is required");
  const document = normalizeSBOM(JSON.parse(readFileSync(path, "utf8")));
  writeFileSync(path, JSON.stringify(document, null, 2) + "\n");
}
