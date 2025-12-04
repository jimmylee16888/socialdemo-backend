package httpx

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"local.dev/socialdemo-backend/internal/models"
)

// GET /boards ；POST /boards
func HandleBoards(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUID(r)

		switch r.Method {
		case http.MethodGet:
			boards := app.Store.ListBoardsFor(uid)
			writeJSON(w, http.StatusOK, boards)

		case http.MethodPost:
			var in struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				IsPrivate   bool   `json:"isPrivate"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			name := strings.TrimSpace(in.Name)
			if name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
				return
			}

			now := time.Now().UTC().Format(time.RFC3339)

			b := models.Board{
				// ⭐ ID 先留空，交給 Store 產
				ID:           "",
				Name:         name,
				Description:  strings.TrimSpace(in.Description),
				OwnerID:      uid,
				ModeratorIDs: []string{},
				IsOfficial:   false,
				IsPrivate:    in.IsPrivate,
				CreatedAt:    now,
				UpdatedAt:    now,
				Deleted:      false,
			}

			// ⭐ 接回回傳值，裡面已經有 ID
			b = app.Store.SaveBoard(b)
			app.Store.SaveBoards(app.Paths.BoardsFile)

			writeJSON(w, http.StatusCreated, b)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

// /boards/{id} 或 /boards/{id}/posts
func HandleBoardSub(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUID(r)
		path := strings.TrimPrefix(r.URL.Path, "/boards/")
		if path == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.SplitN(path, "/", 2)
		boardID := parts[0]

		// /boards/{id}
		if len(parts) == 1 {
			switch r.Method {
			case http.MethodGet:
				b, ok := app.Store.GetBoard(boardID)
				if !ok || b.Deleted {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "board not found"})
					return
				}
				// 私人版但不是 owner → 403
				if b.IsPrivate && b.OwnerID != uid {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
					return
				}
				writeJSON(w, http.StatusOK, b)

			case http.MethodPatch:
				var in struct {
					Name        *string `json:"name"`
					Description *string `json:"description"`
					IsPrivate   *bool   `json:"isPrivate"`
					Deleted     *bool   `json:"deleted"`
				}
				if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
					return
				}

				b, ok := app.Store.GetBoard(boardID)
				if !ok {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "board not found"})
					return
				}
				if b.OwnerID != uid {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "not owner"})
					return
				}

				if in.Name != nil {
					b.Name = strings.TrimSpace(*in.Name)
				}
				if in.Description != nil {
					b.Description = strings.TrimSpace(*in.Description)
				}
				if in.IsPrivate != nil {
					b.IsPrivate = *in.IsPrivate
				}
				if in.Deleted != nil {
					b.Deleted = *in.Deleted
				}
				b.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

				app.Store.SaveBoard(b)
				app.Store.SaveBoards(app.Paths.BoardsFile)

				writeJSON(w, http.StatusOK, b)

			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}

		// /boards/{id}/posts
		if parts[1] == "posts" {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			// 先確認 board 存在且有權限
			b, ok := app.Store.GetBoard(boardID)
			if !ok || b.Deleted {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "board not found"})
				return
			}
			if b.IsPrivate && b.OwnerID != uid {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}

			q := r.URL.Query()
			tagsStr := q.Get("tags")
			beforeStr := q.Get("before")
			limitStr := q.Get("limit")

			var tags []string
			if tagsStr != "" {
				for _, t := range strings.Split(tagsStr, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						tags = append(tags, t)
					}
				}
			}

			var before time.Time
			if beforeStr != "" {
				if t, err := time.Parse(time.RFC3339, beforeStr); err == nil {
					before = t.UTC()
				}
			}
			limit := 0
			if limitStr != "" {
				if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
					limit = n
				}
			}

			// 先用 Store 幫你抓 board 相關貼文（已經依時間排序）
			posts := app.Store.ListByBoard(boardID, tags, uid)

			// 再依 before / limit 做簡單 pagination（不影響沒有傳這些參數的情況）
			if !before.IsZero() {
				var filtered []models.Post
				for _, p := range posts {
					t := parseISO(p.CreatedAt)
					if t.Before(before) {
						filtered = append(filtered, p)
					}
				}
				posts = filtered
			}
			if limit > 0 && len(posts) > limit {
				posts = posts[:limit]
			}

			writeJSON(w, http.StatusOK, posts)
			return
		}

		http.NotFound(w, r)
	}
}
