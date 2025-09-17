// internal/httpx/middleware.go
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"firebase.google.com/go/v4/auth"
	"local.dev/socialdemo-backend/internal/config"
	"local.dev/socialdemo-backend/internal/store"
)

type ctxKey string

const uidKey ctxKey = "uid"

type AppCtx struct {
	Store      *store.Store
	AuthClient *auth.Client
	Paths      config.Paths
}

func currentUID(r *http.Request) string {
	if v := r.Context().Value(uidKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ---- 新增：在 NO_AUTH 時給一個穩定的 Cookie UID ----
const devUIDCookie = "DEV_UID"

func genDevUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "dev_" + hex.EncodeToString(b[:])
}

func devUIDFromCookie(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(devUIDCookie); err == nil && c.Value != "" {
		return c.Value
	}
	id := genDevUID()
	http.SetCookie(w, &http.Cookie{
		Name:     devUIDCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(365 * 24 * time.Hour),
	})
	return id
}

func WithAuth(app *AppCtx, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// NO_AUTH=1：允許 Debug 與 Cookie UID
		if config.NoAuth() {
			uid := ""
			// 1) Authorization: Debug <uid-or-email>
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Debug ") {
				uid = strings.TrimSpace(strings.TrimPrefix(h, "Debug "))
			}
			// 2) 沒有 Debug → 用 Cookie（每個瀏覽器穩定唯一）
			if uid == "" {
				uid = devUIDFromCookie(w, r)
			}
			ctx := context.WithValue(r.Context(), uidKey, uid)
			next(w, r.WithContext(ctx))
			return
		}

		// 正式模式：必須 Bearer <Firebase IdToken>
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		idToken := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		tok, err := app.AuthClient.VerifyIDToken(r.Context(), idToken)
		if err != nil {
			http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), uidKey, tok.UID)
		next(w, r.WithContext(ctx))
	}
}

// 非強制驗證：拿來決定 LikedByMe 等 viewer 身分
func tryViewerUID(app *AppCtx, r *http.Request) string {
	if config.NoAuth() {
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Debug ") {
			return strings.TrimSpace(strings.TrimPrefix(h, "Debug "))
		}
		// 與 WithAuth 一致：用 Cookie 當 viewer
		if c, err := r.Cookie(devUIDCookie); err == nil && c.Value != "" {
			return c.Value
		}
		return ""
	}
	authz := r.Header.Get("Authorization")
	if strings.HasPrefix(authz, "Bearer ") && app.AuthClient != nil {
		idToken := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		if tok, err := app.AuthClient.VerifyIDToken(r.Context(), idToken); err == nil {
			return tok.UID
		}
	}
	return ""
}

func CORS(next http.Handler) http.Handler {
	wrap := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
	return wrap
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
