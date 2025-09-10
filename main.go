// main.go
package main

import (
	"encoding/json"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type User struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	AvatarAsset *string `json:"avatarAsset,omitempty"`
}
type Comment struct {
	ID        string `json:"id"`
	Author    User   `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}
type Post struct {
	ID        string    `json:"id"`
	Author    User      `json:"author"`
	Text      string    `json:"text"`
	CreatedAt string    `json:"createdAt"`
	LikeCount int       `json:"likeCount"`
	LikedByMe bool      `json:"likedByMe"`
	Comments  []Comment `json:"comments"`
	Tags      []string  `json:"tags"`
	ImageURL  *string   `json:"imageUrl,omitempty"` // e.g. "/uploads/xxx.jpg"
}

type Store struct {
	mu    sync.RWMutex
	posts []Post
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

func (s *Store) list(tab string, tags []string) []Post {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Post, 0, len(s.posts))
	if len(tags) > 0 {
		tagset := map[string]struct{}{}
		for _, t := range tags {
			tagset[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
		}
		for _, p := range s.posts {
			for _, pt := range p.Tags {
				if _, ok := tagset[strings.ToLower(pt)]; ok {
					out = append(out, p)
					break
				}
			}
		}
	} else {
		out = append(out, s.posts...)
	}
	if tab == "hot" {
		sort.Slice(out, func(i, j int) bool { return out[i].LikeCount > out[j].LikeCount })
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	}
	return out
}
func (s *Store) create(p Post) Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.posts = append([]Post{p}, s.posts...)
	return p
}
func (s *Store) byID(id string) (Post, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i, p := range s.posts {
		if p.ID == id {
			return p, i
		}
	}
	return Post{}, -1
}
func (s *Store) updateAt(i int, p Post) Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.posts[i] = p
	return p
}
func (s *Store) deleteAt(i int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.posts = append(s.posts[:i], s.posts[i+1:]...)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func ensureDir(d string) { _ = os.MkdirAll(d, 0o755) }

func main() {
	// 路徑與埠由環境變數控制，方便 Render/Fly 等雲端
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	uploadsDir := filepath.Join(dataDir, "uploads")
	ensureDir(uploadsDir)

	store := &Store{}
	// 初始假資料
	store.create(Post{
		ID:        "p1",
		Author:    User{ID: "u_bob", Name: "Bob"},
		Text:      "今天把 UI 卡片邊角修好了 ✅",
		CreatedAt: nowISO(),
		LikeCount: 5,
		Comments:  []Comment{},
		Tags:      []string{"flutter", "design"},
	})
	store.create(Post{
		ID:        "p2",
		Author:    User{ID: "u_me", Name: "Me"},
		Text:      "嗨！這是我的第一篇 🙂",
		CreatedAt: nowISO(),
		LikeCount: 1,
		Comments:  []Comment{},
		Tags:      []string{"hello"},
	})

	mux := http.NewServeMux()

	// 健康檢查
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// CORS（若有 Flutter Web 也可用）
	cors := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	// 靜態檔案（持久化）：/uploads/* -> {DATA_DIR}/uploads/*
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadsDir))))

	// 圖片上傳：POST /upload (form-data: file=<檔案>)
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
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
			if e := strings.ToLower(filepath.Ext(hdr.Filename)); map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}[e] {
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
		path := filepath.Join(uploadsDir, filename)

		out, err := os.Create(path)
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
	})

	// /posts：GET 列表、POST 建立
	mux.HandleFunc("/posts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tab := r.URL.Query().Get("tab")
			var tags []string
			if t := r.URL.Query().Get("tags"); t != "" {
				tags = strings.Split(t, ",")
			}
			writeJSON(w, http.StatusOK, store.list(tab, tags))

		case http.MethodPost:
			var req struct {
				Author   User     `json:"author"`
				Text     string   `json:"text"`
				Tags     []string `json:"tags"`
				ImageURL *string  `json:"imageUrl,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			p := Post{
				ID:     time.Now().Format("20060102T150405.000000000"),
				Author: req.Author, Text: req.Text, CreatedAt: nowISO(),
				LikeCount: 0, Comments: []Comment{}, Tags: req.Tags, ImageURL: req.ImageURL,
			}
			writeJSON(w, http.StatusOK, store.create(p))
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// /posts/{id}, /posts/{id}/like, /posts/{id}/comments
	mux.HandleFunc("/posts/", func(w http.ResponseWriter, r *http.Request) {
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
				var req struct {
					Text     string   `json:"text"`
					Tags     []string `json:"tags"`
					ImageURL *string  `json:"imageUrl,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				p, idx := store.byID(id)
				if idx < 0 {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				p.Text, p.Tags, p.ImageURL = req.Text, req.Tags, req.ImageURL
				writeJSON(w, http.StatusOK, store.updateAt(idx, p))
			case http.MethodDelete:
				p, idx := store.byID(id)
				if idx < 0 {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				if p.ImageURL != nil && strings.HasPrefix(*p.ImageURL, "/uploads/") {
					_ = os.Remove(filepath.Join(".", *p.ImageURL)) // best-effort
				}
				store.deleteAt(idx)
				writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		switch parts[1] {
		case "like":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			p, idx := store.byID(id)
			if idx < 0 {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if p.LikedByMe {
				p.LikedByMe = false
				if p.LikeCount > 0 {
					p.LikeCount--
				}
			} else {
				p.LikedByMe = true
				p.LikeCount++
			}
			writeJSON(w, http.StatusOK, store.updateAt(idx, p))
		case "comments":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Author User   `json:"author"`
				Text   string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			p, idx := store.byID(id)
			if idx < 0 {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			p.Comments = append(p.Comments, Comment{ID: time.Now().Format("20060102T150405.000000000"), Author: req.Author, Text: req.Text, CreatedAt: nowISO()})
			writeJSON(w, http.StatusOK, store.updateAt(idx, p))
		default:
			http.NotFound(w, r)
		}
	})

	// 讀埠（Render 會傳入 $PORT）
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := ":" + port
	log.Println("Server listening on", addr, "DATA_DIR=", dataDir)
	if err := http.ListenAndServe(addr, cors(mux)); err != nil {
		log.Fatal(err)
	}
}
