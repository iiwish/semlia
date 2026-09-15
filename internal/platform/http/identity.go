package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	contract "github.com/iiwish/semlia/api/gen/go"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	identityapp "github.com/iiwish/semlia/internal/application/identity"
	authorization "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	publicid "github.com/iiwish/semlia/pkg/identity"
)

const (
	headerCSRF       = "X-Semlia-CSRF"
	secureCookieName = "__Host-semlia_session"
	devCookieName    = "semlia_session_dev"
)

type IdentityService interface {
	BeginLogin(context.Context, string) (identityapp.LoginStart, error)
	CompleteLogin(context.Context, string, string) (identityapp.LoginComplete, error)
	Authenticate(context.Context, string) (identityapp.AuthenticatedSession, error)
	ValidateCSRF(identityapp.AuthenticatedSession, string) bool
	Logout(context.Context, string) error
	ListMembers(context.Context, publicid.WorkspaceID, string, string) ([]domain.Membership, error)
	ListInvitations(context.Context, publicid.WorkspaceID, string, string) ([]domain.Invitation, error)
	Invite(context.Context, publicid.WorkspaceID, publicid.PrincipalID, identityapp.InvitationInput, string) (identityapp.IssuedInvitation, error)
	SetMembershipStatus(context.Context, publicid.WorkspaceID, publicid.PrincipalID, publicid.MembershipID, domain.MembershipStatus, string) (domain.Membership, error)
}

type IdentityHTTPConfig struct {
	OIDCEnabled    bool
	SecureCookie   bool
	AllowedOrigins []string
}

type authenticatedContextKey struct{}

type requestIdentity struct {
	authenticated identityapp.AuthenticatedSession
	principal     publicid.PrincipalID
	workspace     publicid.WorkspaceID
}

func WithIdentity(service IdentityService, config IdentityHTTPConfig) Option {
	return func(handler *Handler) {
		handler.identity = service
		handler.identityConfig = config
	}
}

func (handler *Handler) authenticateRequest(response http.ResponseWriter, request *http.Request, traceID string, route matchedRoute) (*http.Request, string, bool) {
	if isPublicRoute(route.kind) {
		return request, "", true
	}
	if request.Header.Get("Authorization") != "" {
		for _, name := range []string{secureCookieName, devCookieName} {
			if _, err := request.Cookie(name); err == nil {
				writeIdentityError(response, domain.ErrUnauthenticated, traceID)
				return request, "MIXED_AUTHENTICATION", false
			}
		}
		parts := strings.Fields(request.Header.Get("Authorization"))
		if len(request.Header.Values("Authorization")) != 1 || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || handler.machine == nil || !machineRouteAllowed(route.kind) {
			writeIdentityError(response, domain.ErrUnauthenticated, traceID)
			return request, "UNAUTHENTICATED", false
		}
		machine, err := handler.machine.Authenticate(request.Context(), parts[1])
		if err != nil {
			writeIdentityError(response, err, traceID)
			return request, "UNAUTHENTICATED", false
		}
		if route.workspace != machine.WorkspaceID.String() {
			writeIdentityError(response, domain.ErrForbidden, traceID)
			return request, "FORBIDDEN", false
		}
		if !handler.machineLimits.allow(machine.CredentialID.String(), machine.WorkspaceID.String(), time.Now()) {
			response.Header().Set("Retry-After", "60")
			writeError(response, 429, "RATE_LIMITED", "machine request limit exceeded", traceID, true)
			return request, "RATE_LIMITED", false
		}
		request.Header.Del(headerPrincipal)
		request.Header.Del(headerCSRF)
		request.Header.Del("Authorization")
		ctx := authorizationapp.WithCredentialLimit(request.Context(), machine)
		ctx = context.WithValue(ctx, authenticatedContextKey{}, requestIdentity{principal: machine.PrincipalID, workspace: machine.WorkspaceID})
		return request.WithContext(ctx), "", true
	}
	if handler.identity == nil || isPublicRoute(route.kind) {
		return request, "", true
	}
	cookie, err := request.Cookie(handler.sessionCookieName())
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeIdentityError(response, domain.ErrUnauthenticated, traceID)
		return request, "UNAUTHENTICATED", false
	}
	authenticated, err := handler.identity.Authenticate(request.Context(), cookie.Value)
	if err != nil {
		handler.clearSessionCookie(response)
		writeIdentityError(response, err, traceID)
		return request, "UNAUTHENTICATED", false
	}
	if unsafeMethod(request.Method) {
		if !handler.allowedOrigin(request.Header.Get("Origin")) {
			writeIdentityError(response, domain.ErrForbidden, traceID)
			return request, "INVALID_ORIGIN", false
		}
		if !handler.identity.ValidateCSRF(authenticated, request.Header.Get(headerCSRF)) {
			writeIdentityError(response, domain.ErrForbidden, traceID)
			return request, "INVALID_CSRF", false
		}
	}
	request.Header.Del(headerPrincipal)
	request.Header.Del(headerCSRF)
	identity := requestIdentity{authenticated: authenticated}
	if route.workspace != "" {
		workspaceID, parseErr := publicid.ParseWorkspaceID(route.workspace)
		if parseErr != nil {
			writeIdentityError(response, domain.ErrInvalidArgument, traceID)
			return request, "INVALID_ARGUMENT", false
		}
		membership, ok := activeMembership(authenticated.Session.Memberships, workspaceID)
		if !ok {
			writeIdentityError(response, domain.ErrForbidden, traceID)
			return request, "FORBIDDEN", false
		}
		identity.workspace = workspaceID
		identity.principal = membership.PrincipalID
	}
	ctx := context.WithValue(request.Context(), authenticatedContextKey{}, identity)
	return request.WithContext(ctx), "", true
}

func (handler *Handler) routeIdentity(response http.ResponseWriter, request *http.Request, traceID string, route matchedRoute) string {
	switch route.kind {
	case routeAuthMethods:
		writeJSON(response, http.StatusOK, map[string]bool{"password": true, "oidc": handler.identityConfig.OIDCEnabled})
		return ""
	case routePasswordLogin, routePasswordChange:
		return handler.routePassword(response, request, traceID, route)
	case routeAuthLogin:
		started, err := handler.identity.BeginLogin(request.Context(), request.URL.Query().Get("returnTo"))
		if err != nil {
			return writeIdentityError(response, err, traceID)
		}
		response.Header().Set("Location", started.AuthorizationURL)
		response.WriteHeader(http.StatusFound)
		return ""
	case routeAuthCallback:
		if providerError := request.URL.Query().Get("error"); providerError != "" {
			if redirectAuthFailure(response, request, "provider_error", "身份提供方未完成登录，请重试或联系管理员。") {
				return "OIDC_TRANSACTION"
			}
			return writeIdentityError(response, domain.ErrOIDCTransaction, traceID)
		}
		completed, err := handler.identity.CompleteLogin(request.Context(), request.URL.Query().Get("state"), request.URL.Query().Get("code"))
		if err != nil {
			if redirectAuthFailure(response, request, "callback_failed", "登录回调无效或已经过期，请重新登录。") {
				return "OIDC_TRANSACTION"
			}
			return writeIdentityError(response, err, traceID)
		}
		handler.setSessionCookie(response, completed.SessionToken, completed.ExpiresAt)
		response.Header().Set("Location", completed.ReturnTo)
		response.WriteHeader(http.StatusFound)
		return ""
	case routeSession:
		return handler.routeSession(response, request, traceID)
	case routeWorkspaceMembers, routeWorkspaceInvitations, routeWorkspaceMembership:
		return handler.routeMemberships(response, request, traceID, route)
	default:
		panic("identity route is not handled")
	}
}

func redirectAuthFailure(response http.ResponseWriter, request *http.Request, code, description string) bool {
	if !strings.Contains(strings.ToLower(request.Header.Get("Accept")), "text/html") {
		return false
	}
	query := url.Values{}
	query.Set("auth_error", code)
	query.Set("auth_error_description", description)
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Location", "/?"+query.Encode())
	response.WriteHeader(http.StatusFound)
	return true
}

func (handler *Handler) routeSession(response http.ResponseWriter, request *http.Request, traceID string) string {
	identity, ok := identityFromRequest(request)
	if !ok {
		return writeIdentityError(response, domain.ErrUnauthenticated, traceID)
	}
	if request.Method == http.MethodDelete {
		cookie, _ := request.Cookie(handler.sessionCookieName())
		if cookie != nil {
			if err := handler.identity.Logout(request.Context(), cookie.Value); err != nil {
				return writeIdentityError(response, err, traceID)
			}
		}
		handler.clearSessionCookie(response)
		response.WriteHeader(http.StatusNoContent)
		return ""
	}
	response.Header().Set(headerCSRF, identity.authenticated.CSRFToken)
	workspaces := make([]sessionWorkspaceResponse, 0, len(identity.authenticated.Session.Memberships))
	for _, membership := range identity.authenticated.Session.Memberships {
		capabilities := []contract.AuthorizationAction{}
		roleIDs := append([]string(nil), membership.RoleIDs...)
		authorizationVersion := membership.AuthorizationVersion
		if handler.authorization != nil {
			actions, activeRoleIDs, currentVersion, err := handler.authorization.SessionCapabilities(request.Context(), membership.WorkspaceID, membership.PrincipalID, traceID)
			if err != nil {
				return writeAuthorizationError(response, err, traceID)
			}
			authorizationVersion = currentVersion
			roleIDs = activeRoleIDs
			for _, action := range actions {
				capabilities = append(capabilities, contract.AuthorizationAction(action))
			}
		}
		workspaces = append(workspaces, sessionWorkspaceResponse{
			ID: membership.WorkspaceID.String(), Slug: membership.WorkspaceSlug,
			DisplayName: membership.WorkspaceDisplayName, PrincipalID: membership.PrincipalID.String(),
			RoleIDs: roleIDs, Capabilities: capabilities,
			AuthorizationVersion: authorizationVersion,
		})
	}
	localPassword := false
	if service, ok := handler.identity.(interface {
		HasLocalPassword(context.Context, publicid.UserAccountID) (bool, error)
	}); ok {
		var err error
		localPassword, err = service.HasLocalPassword(request.Context(), identity.authenticated.Session.Account.ID)
		if err != nil {
			return writeIdentityError(response, err, traceID)
		}
	}
	writeJSON(response, http.StatusOK, sessionResponse{
		Account:    sessionAccountResponse{ID: identity.authenticated.Session.Account.ID.String(), DisplayName: identity.authenticated.Session.Account.DisplayName, LocalPassword: localPassword},
		Workspaces: workspaces, ExpiresAt: identity.authenticated.Session.AbsoluteExpiresAt.UTC(), TraceID: traceID,
	})
	return ""
}

func (handler *Handler) routeMemberships(response http.ResponseWriter, request *http.Request, traceID string, route matchedRoute) string {
	identity, ok := identityFromRequest(request)
	if !ok || identity.principal.IsZero() || identity.workspace.IsZero() {
		return writeIdentityError(response, domain.ErrForbidden, traceID)
	}
	switch route.kind {
	case routeWorkspaceMembers:
		if request.Method == http.MethodPost {
			return handler.createPasswordMember(response, request, traceID, identity)
		}
		items, err := handler.identity.ListMembers(request.Context(), identity.workspace, identity.principal.String(), traceID)
		if err != nil {
			return writeIdentityError(response, err, traceID)
		}
		writeJSON(response, http.StatusOK, map[string]any{"items": membershipResponses(items)})
		return ""
	case routeWorkspaceInvitations:
		if request.Method == http.MethodGet {
			items, err := handler.identity.ListInvitations(request.Context(), identity.workspace, identity.principal.String(), traceID)
			if err != nil {
				return writeIdentityError(response, err, traceID)
			}
			writeJSON(response, http.StatusOK, map[string]any{"items": invitationResponses(items)})
			return ""
		}
		var body struct {
			Subject string `json:"subject"`
			Email   string `json:"email"`
			RoleID  string `json:"roleId"`
		}
		if err := decodeRequest(request, &body); err != nil {
			return writeIdentityError(response, domain.ErrInvalidArgument, traceID)
		}
		issued, err := handler.identity.Invite(request.Context(), identity.workspace, identity.principal, identityapp.InvitationInput{Subject: body.Subject, Email: body.Email, RoleID: body.RoleID}, traceID)
		if err != nil {
			return writeIdentityError(response, err, traceID)
		}
		writeJSON(response, http.StatusCreated, invitationResponse(issued.Invitation))
		return ""
	case routeWorkspaceMembership:
		membershipID, err := publicid.ParseMembershipID(route.membership)
		if err != nil {
			return writeIdentityError(response, domain.ErrInvalidArgument, traceID)
		}
		var body struct {
			Status domain.MembershipStatus `json:"status"`
		}
		if err := decodeRequest(request, &body); err != nil {
			return writeIdentityError(response, domain.ErrInvalidArgument, traceID)
		}
		membership, err := handler.identity.SetMembershipStatus(request.Context(), identity.workspace, identity.principal, membershipID, body.Status, traceID)
		if err != nil {
			return writeIdentityError(response, err, traceID)
		}
		writeJSON(response, http.StatusOK, membershipResponse(membership))
		return ""
	}
	panic("membership route is not handled")
}

type sessionResponse struct {
	Account    sessionAccountResponse     `json:"account"`
	Workspaces []sessionWorkspaceResponse `json:"workspaces"`
	ExpiresAt  time.Time                  `json:"expiresAt"`
	TraceID    string                     `json:"traceId"`
}

type sessionAccountResponse struct {
	LocalPassword bool   `json:"localPassword,omitempty"`
	ID            string `json:"id"`
	DisplayName   string `json:"displayName"`
}

type sessionWorkspaceResponse struct {
	ID                   string                         `json:"id"`
	Slug                 string                         `json:"slug"`
	DisplayName          string                         `json:"displayName"`
	PrincipalID          string                         `json:"principalId"`
	RoleIDs              []string                       `json:"roleIds"`
	Capabilities         []contract.AuthorizationAction `json:"capabilities"`
	AuthorizationVersion int64                          `json:"authorizationVersion"`
}

func membershipResponses(items []domain.Membership) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, membershipResponse(item))
	}
	return result
}

func membershipResponse(item domain.Membership) map[string]any {
	return map[string]any{"id": item.ID.String(), "accountId": item.AccountID.String(), "displayName": item.AccountDisplayName, "principalId": item.PrincipalID.String(), "status": item.Status, "roleIds": append([]string{}, item.RoleIDs...), "admittedAt": item.AdmittedAt.UTC()}
}

func invitationResponses(items []domain.Invitation) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, invitationResponse(item))
	}
	return result
}

func invitationResponse(item domain.Invitation) map[string]any {
	return map[string]any{"id": item.ID.String(), "issuer": item.Issuer, "subject": item.Subject, "email": item.Email, "roleId": item.RoleID, "status": item.Status, "expiresAt": item.ExpiresAt.UTC(), "createdAt": item.CreatedAt.UTC()}
}

func identityFromRequest(request *http.Request) (requestIdentity, bool) {
	value, ok := request.Context().Value(authenticatedContextKey{}).(requestIdentity)
	return value, ok
}

func principalRef(request *http.Request) string {
	if identity, ok := identityFromRequest(request); ok {
		return identity.principal.String()
	}
	return strings.TrimSpace(request.Header.Get(headerPrincipal))
}

func activeMembership(items []domain.Membership, workspace publicid.WorkspaceID) (domain.Membership, bool) {
	for _, item := range items {
		if item.WorkspaceID.String() == workspace.String() && item.Status == domain.MembershipActive {
			return item, true
		}
	}
	return domain.Membership{}, false
}

func (handler *Handler) sessionCookieName() string {
	if handler.identityConfig.SecureCookie {
		return secureCookieName
	}
	return devCookieName
}

func (handler *Handler) setSessionCookie(response http.ResponseWriter, value string, expiresAt time.Time) {
	http.SetCookie(response, &http.Cookie{Name: handler.sessionCookieName(), Value: value, Path: "/", HttpOnly: true, Secure: handler.identityConfig.SecureCookie, SameSite: http.SameSiteLaxMode, Expires: expiresAt.UTC(), MaxAge: int(time.Until(expiresAt).Seconds())})
}

func (handler *Handler) clearSessionCookie(response http.ResponseWriter) {
	http.SetCookie(response, &http.Cookie{Name: handler.sessionCookieName(), Value: "", Path: "/", HttpOnly: true, Secure: handler.identityConfig.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
}

func (handler *Handler) allowedOrigin(origin string) bool {
	for _, allowed := range handler.identityConfig.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func unsafeMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func isPublicRoute(kind routeKind) bool {
	return kind == routeSystem || kind == routeAuthLogin || kind == routeAuthCallback || kind == routePasswordLogin || kind == routeAuthMethods
}

func isIdentityRoute(kind routeKind) bool {
	return kind == routeAuthMethods || kind == routePasswordLogin || kind == routePasswordChange || kind == routeAuthLogin || kind == routeAuthCallback || kind == routeSession || kind == routeWorkspaceMembers || kind == routeWorkspaceInvitations || kind == routeWorkspaceMembership
}

func writeIdentityError(response http.ResponseWriter, err error, traceID string) string {
	switch {
	case errors.Is(err, domain.ErrRateLimited):
		response.Header().Set("Retry-After", "300")
		writeError(response, http.StatusTooManyRequests, "RATE_LIMITED", "try signing in again later", traceID, true)
		return "RATE_LIMITED"
	case errors.Is(err, domain.ErrUnauthenticated), errors.Is(err, domain.ErrSessionExpired), errors.Is(err, domain.ErrSessionRevoked):
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication is required", traceID, false)
		return "UNAUTHENTICATED"
	case errors.Is(err, domain.ErrForbidden), errors.Is(err, domain.ErrMembershipInactive), errors.Is(err, domain.ErrAdmissionDenied):
		writeError(response, http.StatusForbidden, "FORBIDDEN", "the authenticated account is not permitted", traceID, false)
		return "FORBIDDEN"
	case errors.Is(err, domain.ErrOIDCTransaction):
		writeError(response, http.StatusBadRequest, "OIDC_TRANSACTION_INVALID", "the sign-in transaction is invalid or expired", traceID, false)
		return "OIDC_TRANSACTION_INVALID"
	case errors.Is(err, domain.ErrInvalidArgument):
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "the request is invalid", traceID, false)
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrNotFound):
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested identity resource was not found", traceID, false)
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrFinalAdministrator):
		writeError(response, http.StatusConflict, "CONFLICT", "the request conflicts with identity policy", traceID, false)
		return "CONFLICT"
	case errors.Is(err, authorization.ErrAuthorizationCeiling), errors.Is(err, authorization.ErrSeparationOfDuties),
		errors.Is(err, authorization.ErrFinalAdministrator), errors.Is(err, authorization.ErrVersionConflict),
		errors.Is(err, authorization.ErrInvariant):
		writeError(response, http.StatusConflict, "CONFLICT", "the request conflicts with authorization policy", traceID, false)
		return "CONFLICT"
	default:
		writeError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "the request could not be completed", traceID, false)
		return "INTERNAL_ERROR"
	}
}
