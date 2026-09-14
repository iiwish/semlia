package httpapi

import (
	"context"
	"mime"
	"net"
	"net/http"

	identityapp "github.com/iiwish/semlia/internal/application/identity"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	publicid "github.com/iiwish/semlia/pkg/identity"
)

type PasswordIdentityService interface {
	PasswordLogin(context.Context, string, string, string) (identityapp.LoginComplete, error)
	ChangePassword(context.Context, publicid.UserAccountID, string, string, string) error
	CreatePasswordMember(context.Context, publicid.WorkspaceID, publicid.PrincipalID, string, string, string, string, string) (domain.Account, error)
}

// Use only the transport peer: forwarded headers are not trusted identity inputs.
func passwordSource(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return host
}

func decodePasswordRequest(response http.ResponseWriter, request *http.Request, body any) error {
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return domain.ErrInvalidArgument
	}
	request.Body = http.MaxBytesReader(response, request.Body, 8192)
	return decodeRequest(request, body)
}

func (handler *Handler) routePassword(response http.ResponseWriter, request *http.Request, traceID string, route matchedRoute) string {
	response.Header().Set("Cache-Control", "no-store")
	service, ok := handler.identity.(PasswordIdentityService)
	if !ok || !handler.allowedOrigin(request.Header.Get("Origin")) {
		return writeIdentityError(response, domain.ErrForbidden, traceID)
	}
	if route.kind == routePasswordLogin {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if decodePasswordRequest(response, request, &body) != nil {
			return writeIdentityError(response, domain.ErrInvalidArgument, traceID)
		}
		completed, err := service.PasswordLogin(request.Context(), body.Username, body.Password, passwordSource(request))
		if err != nil {
			return writeIdentityError(response, err, traceID)
		}
		// An authenticated browser signing in again rotates its existing session.
		if cookie, err := request.Cookie(handler.sessionCookieName()); err == nil {
			if err := handler.identity.Logout(request.Context(), cookie.Value); err != nil {
				_ = handler.identity.Logout(request.Context(), completed.SessionToken)
				return writeIdentityError(response, err, traceID)
			}
		}
		handler.setSessionCookie(response, completed.SessionToken, completed.ExpiresAt)
		response.WriteHeader(http.StatusNoContent)
		return ""
	}
	identity, ok := identityFromRequest(request)
	if !ok {
		return writeIdentityError(response, domain.ErrUnauthenticated, traceID)
	}
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if decodePasswordRequest(response, request, &body) != nil {
		return writeIdentityError(response, domain.ErrInvalidArgument, traceID)
	}
	if err := service.ChangePassword(request.Context(), identity.authenticated.Session.Account.ID, body.CurrentPassword, body.NewPassword, passwordSource(request)); err != nil {
		return writeIdentityError(response, err, traceID)
	}
	handler.clearSessionCookie(response)
	response.WriteHeader(http.StatusNoContent)
	return ""
}

func (handler *Handler) createPasswordMember(response http.ResponseWriter, request *http.Request, traceID string, identity requestIdentity) string {
	response.Header().Set("Cache-Control", "no-store")
	service, ok := handler.identity.(PasswordIdentityService)
	if !ok {
		return writeIdentityError(response, domain.ErrForbidden, traceID)
	}
	var body struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
		RoleID      string `json:"roleId"`
	}
	if decodePasswordRequest(response, request, &body) != nil {
		return writeIdentityError(response, domain.ErrInvalidArgument, traceID)
	}
	account, err := service.CreatePasswordMember(request.Context(), identity.workspace, identity.principal, body.Username, body.DisplayName, body.Password, body.RoleID, traceID)
	if err != nil {
		return writeIdentityError(response, err, traceID)
	}
	writeJSON(response, http.StatusCreated, sessionAccountResponse{ID: account.ID.String(), DisplayName: account.DisplayName})
	return ""
}
