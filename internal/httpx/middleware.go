// internal/httpx/middleware.go
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

const uidKey ctxKey = "uid" // 這裡存的是「身分鍵」：email(小寫) 或 uid 或 dev_xxx

type AppCtx struct {
	Store      *store.Store
	AuthClient *auth.Client
	Paths      config.Paths
}

// === 共用：把 email/uid 正規化成「身分鍵」 ===
func pickKey(email, uid string) string {
	e := strings.TrimSpace(strings.ToLower(email))
	u := strings.TrimSpace(uid)
	if e != "" {
		return e
	}
	if u != "" {
		return u
	}
	return ""
}

func currentUID(r *http.Request) string {
	if v := r.Context().Value(uidKey); v != nil {
		if s, ok := v.(string); ok {
			return s // 回傳的就是「身分鍵」
		}
	}
	return ""
}

// ---- NO_AUTH：Cookie 做為最後保底（每個瀏覽器固定 dev_...）----
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

// ---- NO_AUTH：允許 Bearer，僅解 JWT payload 取出 email/uid（*不驗簽*）----
func devClaimsFromBearer(authz string) (email, uid string) {
	raw := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return "", ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ""
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return "", ""
	}
	get := func(k string) string {
		if v, ok := m[k]; ok && v != nil {
			return strings.TrimSpace(fmt.Sprintf("%v", v))
		}
		return ""
	}
	email = get("email")
	uid = get("user_id")
	if uid == "" {
		uid = get("uid")
	}
	if uid == "" {
		uid = get("sub")
	}
	return email, uid
}

func WithAuth(app *AppCtx, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 免驗證模式：Debug > Bearer(passthrough 只撈 email/uid) > Cookie
		if config.NoAuth() {
			authz := r.Header.Get("Authorization")
			var key string
			switch {
			case strings.HasPrefix(authz, "Debug "):
				// Debug 直接把字串當 key（你可放 email 或自定 uid）
				key = strings.TrimSpace(strings.TrimPrefix(authz, "Debug "))
				if strings.Contains(key, "@") {
					key = strings.ToLower(key) // Debug 也幫你小寫 email
				}
			case strings.HasPrefix(authz, "Bearer "):
				email, uid := devClaimsFromBearer(authz)
				key = pickKey(email, uid)
			}
			if key == "" {
				key = devUIDFromCookie(w, r)
			}
			ctx := context.WithValue(r.Context(), uidKey, key)
			next(w, r.WithContext(ctx))
			return
		}

		// 正式模式：必須 Bearer；用 Firebase 驗證簽章
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
		// 從 claims 撈 email；沒有就用 UID
		email := ""
		if em, ok := tok.Claims["email"].(string); ok {
			email = em
		}
		key := pickKey(email, tok.UID)

		ctx := context.WithValue(r.Context(), uidKey, key)
		next(w, r.WithContext(ctx))
	}
}

// 非強制驗證：給 likedByMe 等 viewer 用（同樣回「身分鍵」）
func tryViewerUID(app *AppCtx, r *http.Request) string {
	if config.NoAuth() {
		authz := r.Header.Get("Authorization")
		if strings.HasPrefix(authz, "Debug ") {
			k := strings.TrimSpace(strings.TrimPrefix(authz, "Debug "))
			if strings.Contains(k, "@") {
				return strings.ToLower(k)
			}
			return k
		}
		if strings.HasPrefix(authz, "Bearer ") {
			email, uid := devClaimsFromBearer(authz)
			if k := pickKey(email, uid); k != "" {
				return k
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
			email := ""
			if em, ok := tok.Claims["email"].(string); ok {
				email = em
			}
			return pickKey(email, tok.UID)
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
