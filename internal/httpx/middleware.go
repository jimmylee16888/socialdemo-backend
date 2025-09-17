package httpx

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// ---- 在 NO_AUTH 模式：Cookie 做為最後保底（每個瀏覽器一把固定 dev_...）----
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

// ---- 在 NO_AUTH 模式：允許 Bearer，僅解 JWT payload 取出 email/uid（*不驗簽*）----
func devUIDFromBearer(authz string) string {
	raw := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return ""
	}
	get := func(k string) string {
		if v, ok := m[k]; ok && v != nil {
			return strings.TrimSpace(fmt.Sprintf("%v", v))
		}
		return ""
	}
	if e := get("email"); e != "" {
		return e
	}
	if u := get("user_id"); u != "" {
		return u
	}
	if u := get("uid"); u != "" {
		return u
	}
	if s := get("sub"); s != "" {
		return s
	}
	return ""
}

func WithAuth(app *AppCtx, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 免驗證模式：優先 Debug，其次 Bearer（解析 payload），最後用 Cookie
		if config.NoAuth() {
			authz := r.Header.Get("Authorization")
			var uid string
			switch {
			case strings.HasPrefix(authz, "Debug "):
				uid = strings.TrimSpace(strings.TrimPrefix(authz, "Debug "))
			case strings.HasPrefix(authz, "Bearer "):
				uid = devUIDFromBearer(authz) // ✅ 讓不同手機用不同 Firebase 帳號 → 不會撞在一起
			}
			if uid == "" {
				uid = devUIDFromCookie(w, r)
			}
			ctx := context.WithValue(r.Context(), uidKey, uid)
			next(w, r.WithContext(ctx))
			return
		}

		// 正式模式：必須帶 Bearer，且用 Firebase 驗證簽章
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

// 非強制驗證：決定 viewer（likedByMe 用）
func tryViewerUID(app *AppCtx, r *http.Request) string {
	if config.NoAuth() {
		authz := r.Header.Get("Authorization")
		if strings.HasPrefix(authz, "Debug ") {
			return strings.TrimSpace(strings.TrimPrefix(authz, "Debug "))
		}
		if strings.HasPrefix(authz, "Bearer ") {
			if uid := devUIDFromBearer(authz); uid != "" {
				return uid
			}
		}
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
