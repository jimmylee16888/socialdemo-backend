package httpx

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"local.dev/socialdemo-backend/internal/models"
)

func HandlePosts(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			viewer := tryViewerUID(app, r)
			tab := r.URL.Query().Get("tab")

			var tags []string
			if t := r.URL.Query().Get("tags"); t != "" {
				tags = strings.Split(t, ",")
			}
			writeJSON(w, http.StatusOK, app.Store.List(tab, tags, viewer))

		case http.MethodPost:
			// 建立貼文：作者=token 身分；前端不需也不能指定作者/暱稱
			WithAuth(app, func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Text     string   `json:"text"`
					Tags     []string `json:"tags"`
					ImageURL *string  `json:"imageUrl,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				uid := currentUID(r)
				p := models.Post{
					ID:        time.Now().Format("20060102T150405.000000000"),
					Author:    models.User{ID: uid, Name: app.Store.DisplayName(uid)},
					Text:      req.Text,
					CreatedAt: time.Now().UTC().Format(time.RFC3339),
					Comments:  []models.Comment{},
					Tags:      req.Tags,
					ImageURL:  req.ImageURL,
				}
				created := app.Store.Create(p)
				app.Store.SavePosts(app.Paths.PostsFile)
				writeJSON(w, http.StatusOK, app.Store.Decorate(created, uid))
			})(w, r)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// /posts/{id}、/posts/{id}/like、/posts/{id}/comments
func HandlePostDetail(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/posts/")
		if path == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(path, "/")
		id := parts[0]

		// /posts/{id}
		if len(parts) == 1 {
			switch r.Method {
			case http.MethodPut:
				WithAuth(app, func(w http.ResponseWriter, r *http.Request) {
					var req struct {
						Text     string   `json:"text"`
						Tags     []string `json:"tags"`
						ImageURL *string  `json:"imageUrl,omitempty"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					p, idx := app.Store.ByID(id)
					if idx < 0 {
						http.Error(w, "not found", http.StatusNotFound)
						return
					}
					// 只有作者本人(或管理員)可編輯
					if currentUID(r) != p.Author.ID && !isAdmin(app, r) {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
					p.Text, p.Tags, p.ImageURL = req.Text, req.Tags, req.ImageURL
					updated := app.Store.UpdateAt(idx, p)
					app.Store.SavePosts(app.Paths.PostsFile)
					writeJSON(w, http.StatusOK, app.Store.Decorate(updated, currentUID(r)))
				})(w, r)

			case http.MethodDelete:
				WithAuth(app, func(w http.ResponseWriter, r *http.Request) {
					p, idx := app.Store.ByID(id)
					if idx < 0 {
						http.Error(w, "not found", http.StatusNotFound)
						return
					}
					// 只有作者本人(或管理員)可刪除
					if currentUID(r) != p.Author.ID && !isAdmin(app, r) {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
					// 刪除本機上傳檔
					if p.ImageURL != nil && strings.HasPrefix(*p.ImageURL, "/uploads/") {
						_ = os.Remove(filepath.Join(app.Paths.UploadsDir, filepath.Base(*p.ImageURL)))
					}
					app.Store.DeleteAt(idx)
					app.Store.SavePosts(app.Paths.PostsFile)
					writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
				})(w, r)

			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		// /posts/{id}/xxx
		switch parts[1] {
		case "like":
			WithAuth(app, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				uid := currentUID(r)
				p, ok := app.Store.ToggleLike(id, uid)
				if !ok {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				app.Store.SaveLikes(app.Paths.LikesFile)
				writeJSON(w, http.StatusOK, app.Store.Decorate(p, uid))
			})(w, r)

		case "comments":
			WithAuth(app, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var req struct {
					Text string `json:"text"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				uid := currentUID(r)
				p, idx := app.Store.ByID(id)
				if idx < 0 {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				p.Comments = append(p.Comments, models.Comment{
					ID:        time.Now().Format("20060102T150405.000000000"),
					Author:    models.User{ID: uid, Name: app.Store.DisplayName(uid)},
					Text:      req.Text,
					CreatedAt: time.Now().UTC().Format(time.RFC3339),
				})
				updated := app.Store.UpdateAt(idx, p)
				app.Store.SavePosts(app.Paths.PostsFile)
				writeJSON(w, http.StatusOK, app.Store.Decorate(updated, uid))
			})(w, r)

		default:
			http.NotFound(w, r)
		}
	}
}

// --- 管理員判斷（目前預設關閉；僅作者可刪/改）。之後要開放可在這裡實作 ---
func isAdmin(_ *AppCtx, _ *http.Request) bool { return false }

// 依前端傳入的好友清單查貼文：POST /posts/query
// body: { "tab": "friends", "friendIds": ["a@x", "demo_bob"], "tags": ["kpop"] }
// internal/httpx/handlers_posts.go
func HandlePostsQuery(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Tab       string   `json:"tab"`
			FriendIDs []string `json:"friendIds"`
			Tags      []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.ToLower(strings.TrimSpace(req.Tab)) != "friends" {
			http.Error(w, "invalid tab (expected 'friends')", http.StatusBadRequest)
			return
		}

		viewer := currentUID(r)
		out := app.Store.ListByAuthors(req.FriendIDs, req.Tags, viewer)

		// ✅ 再保險一次（理論上 ListByAuthors 已保證非 nil）
		if out == nil {
			out = make([]models.Post, 0)
		}
		writeJSON(w, http.StatusOK, out)
	}
}
