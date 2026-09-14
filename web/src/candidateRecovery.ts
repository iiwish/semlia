export interface CandidateRecovery {
  stage: "creating" | "draft" | "submitted";
  proposalId: string;
  assetId: string;
  definition: string;
  reason: string;
  digest: string;
}

const keyFor = (workspaceId: string, candidateId: string) => `semlia:candidate-recovery:v1:${workspaceId}:${candidateId}`;

export function readCandidateRecovery(workspaceId: string, candidateId: string): CandidateRecovery | null {
  const raw = localStorage.getItem(keyFor(workspaceId, candidateId));
  if (!raw) return null;
  const value: unknown = JSON.parse(raw);
  if (!value || typeof value !== "object") throw new Error("候选恢复记录不可读，已暂停提交。请先核对已有提案。");
  const record = value as Record<string, unknown>;
  if (!["creating", "draft", "submitted"].includes(String(record.stage)) ||
    !["proposalId", "assetId", "definition", "reason", "digest"].every((field) => typeof record[field] === "string") ||
    (record.stage !== "creating" && !record.proposalId)) throw new Error("候选恢复记录不完整，已暂停提交。请先核对已有提案。");
  return record as unknown as CandidateRecovery;
}

export function writeCandidateRecovery(workspaceId: string, candidateId: string, value: CandidateRecovery) {
  localStorage.setItem(keyFor(workspaceId, candidateId), JSON.stringify(value));
}

export function clearCandidateRecovery(workspaceId: string, candidateId: string) {
  localStorage.removeItem(keyFor(workspaceId, candidateId));
}

// Keep the checkpoint after association too: a stale candidate in another view
// must reuse the same proposal instead of starting another creation attempt.
const locks = new Map<string, Promise<unknown>>();
export async function withCandidateRecoveryLock<T>(workspaceId: string, candidateId: string, action: () => Promise<T>): Promise<T> {
  const key = keyFor(workspaceId, candidateId);
  if (navigator.locks) return navigator.locks.request(key, action);
  const previous = locks.get(key) ?? Promise.resolve();
  const current = previous.catch(() => undefined).then(action);
  locks.set(key, current);
  try { return await current; }
  finally { if (locks.get(key) === current) locks.delete(key); }
}
