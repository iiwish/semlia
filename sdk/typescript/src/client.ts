import createClient from "openapi-fetch";

import type { paths } from "./schema.gen";

export interface SemliaClientOptions {
  baseUrl: string;
  fetch?: typeof globalThis.fetch;
}

export function createSemliaClient(options: SemliaClientOptions) {
  return createClient<paths>({
    baseUrl: options.baseUrl,
    ...(options.fetch === undefined ? {} : { fetch: options.fetch }),
  });
}

export type SemliaClient = ReturnType<typeof createSemliaClient>;
