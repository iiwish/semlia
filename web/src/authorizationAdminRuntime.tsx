/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

import { AUTHORIZATION_STALE_EVENT } from "./apiClient";
import {
  authorizationAdminApi,
  AuthorizationAdminApiError,
  type AuthorizationAdminApi,
  type AuthorizationMutationResult,
  type CreateAuthorizationBindingInput,
  type CreateAuthorizationRoleInput,
  type InspectAuthorizationInput,
  type RevokeAuthorizationBindingInput,
  type UpdateAuthorizationRoleInput,
} from "./authorizationAdmin";
import type { AuthorizationBinding, AuthorizationDecision, AuthorizationRole } from "./types";

export type AuthorizationAdminDataState = "idle" | "loading" | "ready" | "empty" | "error" | "forbidden";

export interface AuthorizationAdminAccess {
  roleRead: boolean;
  roleManage: boolean;
  roleAssign: boolean;
  authorizationInspect: boolean;
}

interface AuthorizationAdminResourceStatus {
  state: AuthorizationAdminDataState;
  error: string;
  errorCode: string;
}

export interface AuthorizationAdminRuntimeValue {
  workspaceId: string;
  access: AuthorizationAdminAccess;
  roles: AuthorizationRole[];
  bindings: AuthorizationBinding[];
  rolesState: AuthorizationAdminDataState;
  rolesError: string;
  rolesErrorCode: string;
  bindingsState: AuthorizationAdminDataState;
  bindingsError: string;
  bindingsErrorCode: string;
  state: AuthorizationAdminDataState;
  error: string;
  errorCode: string;
  authorizationVersion: string;
  pendingCommand: string;
  refresh: () => Promise<void>;
  refreshRoles: () => Promise<void>;
  refreshBindings: () => Promise<void>;
  createRole: (input: CreateAuthorizationRoleInput) => Promise<AuthorizationRole>;
  updateRole: (roleId: string, input: UpdateAuthorizationRoleInput) => Promise<AuthorizationRole>;
  createBinding: (input: CreateAuthorizationBindingInput) => Promise<AuthorizationBinding>;
  revokeBinding: (bindingId: string, input: RevokeAuthorizationBindingInput) => Promise<AuthorizationBinding>;
  inspect: (input: InspectAuthorizationInput) => Promise<AuthorizationDecision>;
}

export type { AuthorizationAdminApi } from "./authorizationAdmin";

const AuthorizationAdminRuntimeContext = createContext<AuthorizationAdminRuntimeValue | null>(null);

export function AuthorizationAdminRuntimeProvider({
  children,
  workspaceId,
  authorizationVersion: initialAuthorizationVersion = "",
  api: providedApi,
  access,
}: {
  children: ReactNode;
  workspaceId: string;
  authorizationVersion?: string;
  api?: AuthorizationAdminApi;
  access: AuthorizationAdminAccess;
}) {
  const api = providedApi ?? authorizationAdminApi;
  const [roles, setRoles] = useState<AuthorizationRole[]>([]);
  const [bindings, setBindings] = useState<AuthorizationBinding[]>([]);
  const [rolesStatus, setRolesStatus] = useState<AuthorizationAdminResourceStatus>(() => resourceStatus(access.roleRead ? "loading" : "idle"));
  const [bindingsStatus, setBindingsStatus] = useState<AuthorizationAdminResourceStatus>(() => resourceStatus(access.roleAssign ? "loading" : "idle"));
  const [commandError, setCommandError] = useState("");
  const [commandErrorCode, setCommandErrorCode] = useState("");
  const [authorizationVersionState, setAuthorizationVersionState] = useState((initialAuthorizationVersion));
  const [pendingCommand, setPendingCommand] = useState("");

  const loadRoles = useCallback(async (signal?: AbortSignal, throwOnFailure = false) => {
    if (!access.roleRead) {
      setRoles([]);
      setRolesStatus(resourceStatus("idle"));
      return;
    }
    setRolesStatus(resourceStatus("loading"));
    try {
      const nextRoles = await api.listRoles(workspaceId, signal);
      if (signal?.aborted) return;
      setRoles(nextRoles);
      setRolesStatus(resourceStatus(nextRoles.length === 0 ? "empty" : "ready"));
    } catch (reason) {
      if (signal?.aborted) return;
      const detail = errorDetail(reason);
      setRoles([]);
      setRolesStatus({ state: detail.status === 403 ? "forbidden" : "error", error: detail.message, errorCode: detail.code });
      if (throwOnFailure) throw reason;
    }
  }, [access.roleRead, api, workspaceId]);

  const loadBindings = useCallback(async (signal?: AbortSignal, throwOnFailure = false) => {
    if (!access.roleAssign) {
      setBindings([]);
      setBindingsStatus(resourceStatus("idle"));
      return;
    }
    setBindingsStatus(resourceStatus("loading"));
    try {
      const nextBindings = await api.listBindings(workspaceId, signal);
      if (signal?.aborted) return;
      setBindings(nextBindings);
      setBindingsStatus(resourceStatus(nextBindings.length === 0 ? "empty" : "ready"));
    } catch (reason) {
      if (signal?.aborted) return;
      const detail = errorDetail(reason);
      setBindings([]);
      setBindingsStatus({ state: detail.status === 403 ? "forbidden" : "error", error: detail.message, errorCode: detail.code });
      if (throwOnFailure) throw reason;
    }
  }, [access.roleAssign, api, workspaceId]);

  const refresh = useCallback(async () => {
    await Promise.all([loadRoles(), loadBindings()]);
  }, [loadBindings, loadRoles]);

  const refreshRoles = useCallback(() => loadRoles(), [loadRoles]);
  const refreshBindings = useCallback(() => loadBindings(), [loadBindings]);

  useEffect(() => {
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      void loadRoles(controller.signal);
      void loadBindings(controller.signal);
    }, 0);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [loadBindings, loadRoles]);

  const runMutation = useCallback(async <T,>(name: string, allowed: boolean, operation: () => Promise<AuthorizationMutationResult<T>>, refetch: (signal?: AbortSignal, throwOnFailure?: boolean) => Promise<void>): Promise<T> => {
    if (!allowed) throw new AuthorizationAdminApiError("当前会话没有执行此操作的权限。", 403, "FORBIDDEN");
    setPendingCommand(name);
    setCommandError("");
    setCommandErrorCode("");
    try {
      const result = await operation();
      setAuthorizationVersionState(String(result.authorizationVersion));
      await refetch(undefined, true);
      if ((typeof window !== "undefined")) window.dispatchEvent(new CustomEvent(AUTHORIZATION_STALE_EVENT));
      return result.value;
    } catch (reason) {
      const detail = errorDetail(reason);
      setCommandError(detail.message);
      setCommandErrorCode(detail.code);
      throw reason;
    } finally {
      setPendingCommand("");
    }
  }, []);

  const createRole = useCallback((input: CreateAuthorizationRoleInput) =>
    runMutation("create-role", access.roleManage, () => api.createRole(workspaceId, input), loadRoles), [access.roleManage, api, loadRoles, runMutation, workspaceId]);
  const updateRole = useCallback((roleId: string, input: UpdateAuthorizationRoleInput) =>
    runMutation("update-role", access.roleManage, () => api.updateRole(workspaceId, roleId, input), loadRoles), [access.roleManage, api, loadRoles, runMutation, workspaceId]);
  const createBinding = useCallback((input: CreateAuthorizationBindingInput) =>
    runMutation("create-binding", access.roleAssign, () => api.createBinding(workspaceId, input), loadBindings), [access.roleAssign, api, loadBindings, runMutation, workspaceId]);
  const revokeBinding = useCallback((bindingId: string, input: RevokeAuthorizationBindingInput) =>
    runMutation("revoke-binding", access.roleAssign, () => api.revokeBinding(workspaceId, bindingId, input), loadBindings), [access.roleAssign, api, loadBindings, runMutation, workspaceId]);
  const inspect = useCallback(async (input: InspectAuthorizationInput) => {
    if (!access.authorizationInspect) throw new AuthorizationAdminApiError("当前会话没有有效权限检查能力。", 403, "FORBIDDEN");
    setPendingCommand("inspect");
    setCommandError("");
    setCommandErrorCode("");
    try {
      const decision = await api.inspect(workspaceId, input);
      setAuthorizationVersionState(decision.authorizationVersion);
      return decision;
    } catch (reason) {
      const detail = errorDetail(reason);
      setCommandError(detail.message);
      setCommandErrorCode(detail.code);
      throw reason;
    } finally {
      setPendingCommand("");
    }
  }, [access.authorizationInspect, api, workspaceId]);

  const requestedStatuses = [access.roleRead ? rolesStatus : null, access.roleAssign ? bindingsStatus : null].filter((item): item is AuthorizationAdminResourceStatus => item !== null);
  const state = combinedState(requestedStatuses);
  const resourceFailure = requestedStatuses.find((item) => item.state === "forbidden" || item.state === "error");
  const error = commandError || resourceFailure?.error || "";
  const errorCode = commandErrorCode || resourceFailure?.errorCode || "";

  const value = useMemo<AuthorizationAdminRuntimeValue>(() => ({
    workspaceId,
    access,
    roles,
    bindings,
    rolesState: rolesStatus.state,
    rolesError: rolesStatus.error,
    rolesErrorCode: rolesStatus.errorCode,
    bindingsState: bindingsStatus.state,
    bindingsError: bindingsStatus.error,
    bindingsErrorCode: bindingsStatus.errorCode,
    state,
    error,
    errorCode,
    authorizationVersion: authorizationVersionState,
    pendingCommand,
    refresh,
    refreshRoles,
    refreshBindings,
    createRole,
    updateRole,
    createBinding,
    revokeBinding,
    inspect,
  }), [access, authorizationVersionState, bindings, bindingsStatus, createBinding, createRole, error, errorCode, inspect, pendingCommand, refresh, refreshBindings, refreshRoles, revokeBinding, roles, rolesStatus, state, updateRole, workspaceId]);

  return <AuthorizationAdminRuntimeContext.Provider value={value}>{children}</AuthorizationAdminRuntimeContext.Provider>;
}

export function useAuthorizationAdminRuntime(): AuthorizationAdminRuntimeValue {
  const value = useContext(AuthorizationAdminRuntimeContext);
  if (!value) throw new Error("useAuthorizationAdminRuntime must be used inside AuthorizationAdminRuntimeProvider");
  return value;
}

function errorDetail(reason: unknown): { message: string; code: string; status: number } {
  if (reason && typeof reason === "object") {
    const message = "message" in reason && typeof reason.message === "string" ? reason.message : "访问控制请求失败。";
    const code = "code" in reason && typeof reason.code === "string" ? reason.code : "REQUEST_FAILED";
    const status = "status" in reason && typeof reason.status === "number" ? reason.status : 0;
    return { message, code, status };
  }
  return { message: "访问控制请求失败。", code: "REQUEST_FAILED", status: 0 };
}

function resourceStatus(state: AuthorizationAdminDataState): AuthorizationAdminResourceStatus {
  return { state, error: "", errorCode: "" };
}

function combinedState(statuses: AuthorizationAdminResourceStatus[]): AuthorizationAdminDataState {
  if (statuses.length === 0) return "ready";
  if (statuses.some((item) => item.state === "loading")) return "loading";
  if (statuses.some((item) => item.state === "forbidden")) return "forbidden";
  if (statuses.some((item) => item.state === "error")) return "error";
  if (statuses.every((item) => item.state === "empty")) return "empty";
  return "ready";
}
