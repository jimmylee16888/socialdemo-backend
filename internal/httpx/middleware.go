package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

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

func WithAuth(app *AppCtx, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if config.NoAuth() {
			// 免驗證：Authorization: Debug <uid>
			uid := "u_me"
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Debug ") {
				uid = strings.TrimSpace(strings.TrimPrefix(h, "Debug "))
			}
			ctx := context.WithValue(r.Context(), uidKey, uid)
			next(w, r.WithContext(ctx))
			return
		}
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

// 非強制驗證：若帶 token 就解析 viewer；NO_AUTH=1 可用 Debug <uid>
func tryViewerUID(app *AppCtx, r *http.Request) string {
	if config.NoAuth() {
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Debug ") {
			return strings.TrimSpace(strings.TrimPrefix(h, "Debug "))
		}
		return ""
	}
	authz := r.Header.Get("Authorization")
	if strings.HasPrefix(authz, "Bearer ") {
		idToken := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		if tok, err := app.AuthClient.VerifyIDToken(r.Context(), idToken); err == nil {
			return tok.UID
		}
	}
	return ""
}

// CORS
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// 僅保護性刪檔（上傳在本機時）
func removeUploadIfLocal(path string) {
	if path == "" {
		return
	}
	if strings.HasPrefix(path, "/uploads/") {
		_ = os.Remove(strings.TrimPrefix(path, "/"))
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
