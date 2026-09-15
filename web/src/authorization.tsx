/* eslint-disable react-refresh/only-export-components */
import { createContext, useContext, useMemo, type ReactNode } from "react";

import type {
  AuthorizationBinding,
  AuthorizationDecision,
  AuthorizationResource,
  AuthorizationRole,
  AuthorizationScope,
  AuthorizationPrincipal,
  CapabilitySession,
  PermissionAction,
  SeparationOfDutyConflict,
} from "./types";

interface CapabilityContextValue {
  session: CapabilitySession;
  can: (action: PermissionAction) => boolean;
  decide: (action: PermissionAction, resource?: AuthorizationResource) => AuthorizationDecision;
}

interface DecisionInput {
  principalId: string;
  action: PermissionAction;
  resource?: AuthorizationResource;
  principals?: AuthorizationPrincipal[];
  roles?: AuthorizationRole[];
  bindings?: AuthorizationBinding[];
  authorizationVersion?: string;
}

interface SeparationOfDutyInput {
  principalId: string;
  roleId: string;
  scope: AuthorizationScope;
  roles?: AuthorizationRole[];
  bindings?: AuthorizationBinding[];
}

const CapabilityContext = createContext<CapabilityContextValue | null>(null);

function scopeMatches(scope: AuthorizationScope, resource?: AuthorizationResource) {
  if (!resource || scope.type === "workspace") return true;
  if (scope.type === resource.type && scope.id === resource.id) return true;
  if (scope.type === "domain" && scope.id === resource.domainId) return true;
  if (scope.type === "environment" && scope.id === resource.environment) return true;
  return false;
}

export function evaluateAuthorization({
  principalId,
  action,
  resource,
  principals = [],
  roles = [],
  bindings = [],
  authorizationVersion = "",
}: DecisionInput): AuthorizationDecision {
  const principal = principals.find((item) => item.id === principalId);
  if (!principal || principal.status !== "active") {
    return {
      allowed: false,
      action,
      principalId,
      reasonCode: "PRINCIPAL_INACTIVE",
      explanation: "主体不存在或已停用，不能获得任何权限。",
      authorizationVersion,
    };
  }

  const matchingBinding = bindings.find((binding) => {
    if (binding.principalId !== principalId || binding.status !== "active" || !scopeMatches(binding.scope, resource)) return false;
    return roles.find((role) => role.id === binding.roleId)?.permissions.includes(action);
  });

  if (!matchingBinding) {
    return {
      allowed: false,
      action,
      principalId,
      reasonCode: "NO_MATCHING_GRANT",
      explanation: `没有找到覆盖当前资源的 ${action} 授权。`,
      authorizationVersion,
    };
  }

  const role = roles.find((item) => item.id === matchingBinding.roleId);
  return {
    allowed: true,
    action,
    principalId,
    reasonCode: "ROLE_GRANT",
    explanation: `${role?.name ?? matchingBinding.roleId} 通过 ${matchingBinding.scope.label} 范围授予此操作。`,
    authorizationVersion,
    roleId: role?.id,
    bindingId: matchingBinding.id,
    scope: matchingBinding.scope,
  };
}

export function findSeparationOfDutyConflicts({
  principalId,
  roleId,
  scope,
  roles = [],
  bindings = [],
}: SeparationOfDutyInput): SeparationOfDutyConflict[] {
  if (!scope.protected) return [];
  const role = roles.find((item) => item.id === roleId);
  if (!role?.incompatibleRoleIds?.length) return [];

  const conflictingBinding = bindings.find((binding) => {
    if (binding.principalId !== principalId || binding.status !== "active") return false;
    if (!role.incompatibleRoleIds?.includes(binding.roleId)) return false;
    return binding.scope.type === "workspace" || binding.scope.id === scope.id || binding.scope.protected;
  });

  if (!conflictingBinding) return [];
  return [
    {
      code: "PROTECTED_REVIEW_PUBLISH_CONFLICT",
      message: "受保护范围的评审者与发布者必须相互独立，请选择其他主体或缩小授权范围。",
      blocking: true,
      conflictingBindingId: conflictingBinding.id,
    },
  ];
}

const unauthenticatedSession: CapabilitySession = { principalId: "", version: "", capabilities: [] };

export function CapabilityProvider({ children, session = unauthenticatedSession }: { children: ReactNode; session?: CapabilitySession }) {
  const value = useMemo<CapabilityContextValue>(
    () => ({
      session,
      can: (action) => session.capabilities.includes(action),
      decide: (action) => {
        const allowed = session.capabilities.includes(action);
        return {
          allowed,
          action,
          principalId: session.principalId,
          reasonCode: allowed ? "SESSION_CAPABILITY" : "NO_MATCHING_GRANT",
          explanation: allowed ? `当前会话包含 ${action} 能力；具体资源仍由服务端复核。` : `当前会话不包含 ${action} 能力。`,
          authorizationVersion: session.version,
        };
      },
    }),
    [session],
  );

  return <CapabilityContext.Provider value={value}>{children}</CapabilityContext.Provider>;
}

function useCapabilityContext() {
  const value = useContext(CapabilityContext);
  if (!value) throw new Error("Authorization hooks must be used inside CapabilityProvider.");
  return value;
}

export function useCan(action: PermissionAction) {
  return useCapabilityContext().can(action);
}

export function useResourceCan(action: PermissionAction, resource: AuthorizationResource) {
  return useCapabilityContext().decide(action, resource);
}

export function RequireCapability({
  action,
  resource,
  fallback = null,
  children,
}: {
  action: PermissionAction;
  resource?: AuthorizationResource;
  fallback?: ReactNode;
  children: ReactNode;
}) {
  const context = useCapabilityContext();
  const allowed = resource ? context.decide(action, resource).allowed : context.can(action);
  return allowed ? children : fallback;
}
