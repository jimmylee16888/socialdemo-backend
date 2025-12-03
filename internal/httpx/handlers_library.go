package httpx

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// 我們不強制 schema，直接把 body 解成 map[string]any
type LibraryPayload map[string]any

// /api/v1/library/sync
//
// 流程：
// 1) 用 currentUID(r) 拿到這個 user 的 key（email/uid/dev_xxx）
// 2) 讀取 body，確認是合法 JSON
// 3) 存一份快照到 data/library_<uid>.json
// 4) 把 payload 原封不動回傳給 App（讓 Flutter 的 _applyMergedResult 吃）
func HandleLibrarySync(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 1) 這個 user 的身分鍵（跟 HandleMe / HandleMyTags 一樣）
		uid := currentUID(r)
		if uid == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// 2) 讀 body（順便限制最大 2MB）
		defer r.Body.Close()
		const maxBody = 2 << 20 // 2MB
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
		if err != nil {
			http.Error(w, "read body failed", http.StatusBadRequest)
			return
		}

		// 3) 確認是合法 JSON（但不檢查欄位內容）
		var payload LibraryPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		// 4) 存檔：data/library_<uid>.json
		filename := "library_" + uid + ".json"
		path := filepath.Join(app.Paths.DataDir, filename)

		wrapped := map[string]any{
			"user_id":    uid,
			"updated_at": time.Now().UTC().Format(time.RFC3339),
			"payload":    payload,
		}

		data, err := json.MarshalIndent(wrapped, "", "  ")
		if err != nil {
			log.Printf("[library-sync] marshal error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if err := os.WriteFile(path, data, 0o644); err != nil {
			log.Printf("[library-sync] write file %s error: %v", path, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		log.Printf("[library-sync] saved library for uid=%s file=%s", uid, path)

		// 5) 回傳 payload 本體（不是 wrapped），對齊 Flutter 目前期待的格式：
		// {
		//   "card_item_store": {...},
		//   "mini_card_store": {...},
		//   "albums": [...]
		// }
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			log.Printf("[library-sync] write response error: %v", err)
		}
	}
}

// /api/v1/library/snapshot
//
// 流程：
// 1) 用 currentUID(r) 拿 user key
// 2) 讀 data/library_<uid>.json
// 3) 把裡面的 "payload" 原封不動回給 App
func HandleLibrarySnapshot(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		uid := currentUID(r)
		if uid == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		filename := "library_" + uid + ".json"
		path := filepath.Join(app.Paths.DataDir, filename)

		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				// 還沒 sync 過：回 404，讓前端知道「沒有雲端 snapshot」
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			log.Printf("[library-snapshot] read file %s error: %v", path, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		var wrapped map[string]any
		if err := json.Unmarshal(data, &wrapped); err != nil {
			log.Printf("[library-snapshot] unmarshal error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		payload, ok := wrapped["payload"]
		if !ok {
			log.Printf("[library-snapshot] missing payload field in %s", path)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			log.Printf("[library-snapshot] write response error: %v", err)
		}
	}
}
