package httpx

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"local.dev/socialdemo-backend/internal/models"
)

// ---- Upload ----
func HandleUpload(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 20<<20) // 20MB
		if err := r.ParseMultipartForm(25 << 20); err != nil {
			http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "form file: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		head := make([]byte, 512)
		n, _ := io.ReadFull(file, head)
		head = head[:n]
		mtype := http.DetectContentType(head)

		ext := ""
		switch mtype {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/webp":
			ext = ".webp"
		case "image/gif":
			ext = ".gif"
		default:
			if e := strings.ToLower(filepath.Ext(hdr.Filename)); map[string]bool{
				".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
			}[e] {
				ext = e
			}
			if ext == "" {
				http.Error(w, "unsupported image type: "+mtype, http.StatusBadRequest)
				return
			}
		}

		ts := time.Now().Format("20060102T150405.000")
		base := strings.TrimSuffix(hdr.Filename, filepath.Ext(hdr.Filename))
		if base == "" {
			base = "img"
		}
		base = strings.Map(func(r rune) rune {
			if r == '-' || r == '_' || r == '.' || r == ' ' ||
				(r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				return r
			}
			return '-'
		}, base)
		filename := ts + "_" + base + ext
		dst := filepath.Join(app.Paths.UploadsDir, filename)

		out, err := os.Create(dst)
		if err != nil {
			http.Error(w, "create file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer out.Close()
		if _, err := out.Write(head); err != nil {
			http.Error(w, "write head: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := io.Copy(out, file); err != nil {
			http.Error(w, "write file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if ctype := mime.TypeByExtension(ext); ctype != "" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": "/uploads/" + filename})
	}
}

// ---- Posts ----
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
					if currentUID(r) != p.Author.ID {
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
					if currentUID(r) != p.Author.ID {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
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

// ---- Me / Tags / Friends ----
func HandleMe(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUID(r)
		switch r.Method {
		case http.MethodGet:
			if p, ok := app.Store.GetProfile(uid); ok {
				writeJSON(w, http.StatusOK, p)
				return
			}
			writeJSON(w, http.StatusOK, models.Profile{ID: uid, Name: uid})
		case http.MethodPatch:
			var p models.Profile
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			p.ID = uid
			updated := app.Store.UpsertProfile(p)
			app.Store.SaveProfiles(app.Paths.ProfilesFile)
			writeJSON(w, http.StatusOK, updated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func HandleMyTags(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUID(r)
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, app.Store.GetTags(uid))
		case http.MethodPost:
			var body struct {
				Tag string `json:"tag"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			tags := app.Store.AddTag(uid, body.Tag)
			app.Store.SaveTags(app.Paths.TagsFile)
			writeJSON(w, http.StatusOK, tags)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func HandleMyTagsDelete(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		uid := currentUID(r)
		tag := strings.TrimPrefix(r.URL.Path, "/me/tags/")
		if tag == "" {
			http.NotFound(w, r)
			return
		}
		tags := app.Store.RemoveTag(uid, tag)
		app.Store.SaveTags(app.Paths.TagsFile)
		writeJSON(w, http.StatusOK, tags)
	}
}

func HandleMyFriends(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		uid := currentUID(r)
		writeJSON(w, http.StatusOK, app.Store.GetFriends(uid))
	}
}

// ---- Users ----
func HandleUsers(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/users/")
		if rest == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(rest, "/")
		userId := parts[0]

		if len(parts) == 1 {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if p, ok := app.Store.GetProfile(userId); ok {
				writeJSON(w, http.StatusOK, p)
				return
			}
			writeJSON(w, http.StatusOK, models.Profile{ID: userId, Name: userId})
			return
		}

		switch parts[1] {
		case "posts":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			viewer := tryViewerUID(app, r)
			writeJSON(w, http.StatusOK, app.Store.UserPosts(userId, viewer))

		case "follow":
			WithAuth(app, func(w http.ResponseWriter, r *http.Request) {
				uid := currentUID(r)
				switch r.Method {
				case http.MethodPost:
					app.Store.Follow(uid, userId)
					app.Store.SaveFriends(app.Paths.FriendsFile)
					w.WriteHeader(http.StatusNoContent)
				case http.MethodDelete:
					app.Store.Unfollow(uid, userId)
					app.Store.SaveFriends(app.Paths.FriendsFile)
					w.WriteHeader(http.StatusNoContent)
				default:
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
			})(w, r)

		default:
			http.NotFound(w, r)
		}
	}
}
