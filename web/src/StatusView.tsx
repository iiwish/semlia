import { useEffect, useState } from "react";
import {
  Activity,
  Check,
  CircleAlert,
  Database,
  LoaderCircle,
  RefreshCw,
  Server,
  ShieldAlert,
} from "lucide-react";

import {
  loadSystemStatus,
  loadingStatus,
  type StatusLoader,
  type SystemStatus,
} from "./status";
import "./status.css";

interface AppProps {
  loadStatus?: StatusLoader;
}

type Tone = "loading" | "ready" | "warning" | "danger" | "neutral";

export function StatusView({ loadStatus = loadSystemStatus }: AppProps) {
  const [status, setStatus] = useState<SystemStatus>(loadingStatus);
  const [requestVersion, setRequestVersion] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    loadStatus(controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) {
          setStatus(result);
        }
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setStatus({
            kind: "configuration_error",
            code: "CONFIGURATION_ERROR",
            message: "Unable to reach the control API.",
            traceId: null,
          });
        }
      });
    return () => controller.abort();
  }, [loadStatus, requestVersion]);

  const refresh = () => {
    setStatus(loadingStatus);
    setRequestVersion((version) => version + 1);
  };

  const presentation = present(status);
  const info = status.kind === "ready" ? status.info : status.kind === "dependency_unavailable" ? status.info : undefined;

  return (
    <div className="system-status-root" data-state={status.kind}>
      <header className="topbar">
        <div className="topbar-inner">
          <a className="brand" href="/" aria-label="Semlia system status">
            <span className="brand-mark" aria-hidden="true">S</span>
            <span>Semlia</span>
          </a>
          <span className="environment-label">Control plane</span>
        </div>
      </header>

      <main className="status-page">
        <div className="page-heading">
          <div>
            <p className="eyebrow">Instance diagnostics</p>
            <h1>System status</h1>
            <p className="page-description">Live health and contract information from this Semlia instance.</p>
          </div>
          <button
            className="refresh-button"
            type="button"
            onClick={refresh}
            disabled={status.kind === "loading"}
            aria-label="Refresh status"
          >
            <RefreshCw size={16} aria-hidden="true" />
            <span>Refresh</span>
          </button>
        </div>

        <section
          className={`state-banner tone-${presentation.tone}`}
          role={presentation.problem ? "alert" : "status"}
          aria-live="polite"
          aria-atomic="true"
        >
          <StateIcon status={status} />
          <div className="state-copy">
            <strong>{presentation.title}</strong>
            <span>{presentation.description}</span>
          </div>
          <span className="state-code">{presentation.code}</span>
        </section>

        <section className="signal-section" aria-labelledby="service-path-title">
          <div className="section-heading-row">
            <div>
              <p className="section-kicker">Runtime path</p>
              <h2 id="service-path-title">Service Signal Line</h2>
            </div>
            <span className="last-check">On demand</span>
          </div>

          <div className="signal-line" data-tone={presentation.tone}>
            <SignalNode
              icon={<Activity size={18} aria-hidden="true" />}
              label="Browser"
              value="Connected"
              tone="ready"
            />
            <span className="signal-connector" aria-hidden="true" />
            <SignalNode
              icon={<Server size={18} aria-hidden="true" />}
              label="Control API"
              value={apiSignal(status)}
              tone={apiTone(status)}
            />
            <span className="signal-connector" aria-hidden="true" />
            <SignalNode
              icon={<Database size={18} aria-hidden="true" />}
              label="Persistence"
              value={dependencySignal(status)}
              tone={dependencyTone(status)}
            />
          </div>
        </section>

        <section className="details-section" aria-labelledby="details-title">
          <div className="section-heading-row">
            <div>
              <p className="section-kicker">Current reading</p>
              <h2 id="details-title">Instance details</h2>
            </div>
          </div>

          <div className="status-grid">
            <StatusCard
              label="Process"
              value={processValue(status)}
              detail={status.kind === "configuration_error" ? "No live response" : "Liveness endpoint"}
              tone={processTone(status)}
            />
            <StatusCard
              label="Dependency"
              value={dependencySignal(status)}
              detail={status.kind === "dependency_unavailable" && status.retryable ? "Retry is supported" : "Readiness endpoint"}
              tone={dependencyTone(status)}
            />
            <StatusCard
              label="Contract"
              value={info ? `${info.apiVersion} / ${info.schemaVersion}` : status.kind === "loading" ? "Checking" : "Unavailable"}
              detail={info ? info.buildVersion : "Build information"}
              tone={info ? "ready" : status.kind === "loading" ? "loading" : "neutral"}
              mono
            />
          </div>
        </section>

        <section className="trace-section" aria-labelledby="trace-title">
          <div>
            <p className="section-kicker">Support context</p>
            <h2 id="trace-title">Trace ID</h2>
          </div>
          <code>{traceValue(status)}</code>
        </section>
      </main>
    </div>
  );
}

function StateIcon({ status }: { status: SystemStatus }) {
  if (status.kind === "loading") {
    return <LoaderCircle className="state-icon is-spinning" size={22} aria-hidden="true" />;
  }
  if (status.kind === "ready") {
    return <Check className="state-icon" size={22} aria-hidden="true" />;
  }
  if (status.kind === "dependency_unavailable") {
    return <CircleAlert className="state-icon" size={22} aria-hidden="true" />;
  }
  return <ShieldAlert className="state-icon" size={22} aria-hidden="true" />;
}

function SignalNode({
  icon,
  label,
  value,
  tone,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  tone: Tone;
}) {
  return (
    <div className="signal-node" data-tone={tone}>
      <span className="node-icon">{icon}</span>
      <span className="node-copy">
        <strong>{label}</strong>
        <span>{value}</span>
      </span>
      <span className="node-indicator" aria-hidden="true" />
    </div>
  );
}

function StatusCard({
  label,
  value,
  detail,
  tone,
  mono = false,
}: {
  label: string;
  value: string;
  detail: string;
  tone: Tone;
  mono?: boolean;
}) {
  return (
    <article className="status-card" data-tone={tone}>
      <div className="card-label-row">
        <span className="card-indicator" aria-hidden="true" />
        <span>{label}</span>
      </div>
      <strong className={mono ? "mono" : undefined}>{value}</strong>
      <span className="card-detail">{detail}</span>
    </article>
  );
}

function present(status: SystemStatus): {
  title: string;
  description: string;
  code: string;
  tone: Tone;
  problem: boolean;
} {
  switch (status.kind) {
    case "loading":
      return {
        title: "Checking control plane",
        description: "Reading process, dependency and contract health.",
        code: "CHECKING",
        tone: "loading",
        problem: false,
      };
    case "ready":
      return {
        title: "Control plane ready",
        description: "The process and required dependencies report healthy.",
        code: "READY",
        tone: "ready",
        problem: false,
      };
    case "dependency_unavailable":
      return {
        title: "Dependency unavailable",
        description: status.message,
        code: status.code,
        tone: "warning",
        problem: true,
      };
    case "configuration_error":
      return {
        title: "Control API unreachable",
        description: status.message,
        code: status.code,
        tone: "danger",
        problem: true,
      };
  }
}

function apiSignal(status: SystemStatus): string {
  if (status.kind === "loading") return "Checking";
  if (status.kind === "configuration_error") return "Unreachable";
  return "Live";
}

function apiTone(status: SystemStatus): Tone {
  if (status.kind === "loading") return "loading";
  if (status.kind === "configuration_error") return "danger";
  return "ready";
}

function dependencySignal(status: SystemStatus): string {
  switch (status.kind) {
    case "loading": return "Checking";
    case "ready": return "Ready";
    case "dependency_unavailable": return "Unavailable";
    case "configuration_error": return "Unknown";
  }
}

function dependencyTone(status: SystemStatus): Tone {
  switch (status.kind) {
    case "loading": return "loading";
    case "ready": return "ready";
    case "dependency_unavailable": return "warning";
    case "configuration_error": return "neutral";
  }
}

function processValue(status: SystemStatus): string {
  if (status.kind === "loading") return "Checking";
  if (status.kind === "configuration_error") return "Unknown";
  return "Live";
}

function processTone(status: SystemStatus): Tone {
  if (status.kind === "loading") return "loading";
  if (status.kind === "configuration_error") return "neutral";
  return "ready";
}

function traceValue(status: SystemStatus): string {
  if (status.kind === "loading") return "Pending response";
  return status.traceId ?? "Not available";
}
