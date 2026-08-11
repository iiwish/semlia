import {
  createSemliaClient,
  type components,
  type SemliaClient,
} from "@semlia/sdk-typescript";

type SystemInfo = components["schemas"]["SystemInfo"];
type ErrorResponse = components["schemas"]["ErrorResponse"];

export type SystemStatus =
  | { kind: "loading" }
  | { kind: "ready"; traceId: string; info: SystemInfo }
  | {
      kind: "dependency_unavailable";
      code: string;
      message: string;
      traceId: string;
      retryable: boolean;
      info?: SystemInfo;
    }
  | {
      kind: "configuration_error";
      code: string;
      message: string;
      traceId: string | null;
    };

export type StatusLoader = (signal?: AbortSignal) => Promise<SystemStatus>;

export const loadingStatus: SystemStatus = { kind: "loading" };

const browserClient = createSemliaClient({ baseUrl: "" });

export const loadSystemStatus: StatusLoader = (signal) =>
  loadSystemStatusWithClient(browserClient, signal);

export async function loadSystemStatusWithClient(
  client: SemliaClient,
  signal?: AbortSignal,
): Promise<SystemStatus> {
  try {
    const liveness = await client.GET("/health/live", { signal });
    if (!liveness.data || liveness.data.status !== "live") {
      return configurationError(liveness.error);
    }

    const [readiness, systemInfo] = await Promise.all([
      client.GET("/health/ready", { signal }),
      client.GET("/api/v1/system/info", { signal }),
    ]);

    if (readiness.error && isErrorResponse(readiness.error)) {
      return {
        kind: "dependency_unavailable",
        code: readiness.error.code,
        message: readiness.error.message,
        traceId: readiness.error.traceId,
        retryable: readiness.error.retryable ?? false,
        ...(systemInfo.data === undefined ? {} : { info: systemInfo.data }),
      };
    }

    if (
      !readiness.data ||
      readiness.data.status !== "ready" ||
      !systemInfo.data
    ) {
      return configurationError(systemInfo.error ?? readiness.error);
    }

    return {
      kind: "ready",
      traceId: systemInfo.data.traceId,
      info: systemInfo.data,
    };
  } catch (error) {
    if (signal?.aborted) {
      throw error;
    }
    return configurationError(error);
  }
}

function configurationError(value: unknown): SystemStatus {
  if (isErrorResponse(value)) {
    return {
      kind: "configuration_error",
      code: value.code,
      message: value.message,
      traceId: value.traceId,
    };
  }
  return {
    kind: "configuration_error",
    code: "CONFIGURATION_ERROR",
    message: "Unable to reach the control API.",
    traceId: null,
  };
}

function isErrorResponse(value: unknown): value is ErrorResponse {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as Partial<ErrorResponse>;
  return (
    typeof candidate.code === "string" &&
    typeof candidate.message === "string" &&
    typeof candidate.traceId === "string" &&
    candidate.details !== undefined
  );
}
