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

// ======== Models ========

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

// 供 /users/:id 與 /me 使用的公開/半公開資料
type Profile struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`      // 系統名稱
	Nickname      *string `json:"nickname"`  // 顯示暱稱（可為空）
	AvatarURL     *string `json:"avatarUrl"` // 可為相對路徑
	Instagram     *string `json:"instagram"`
	Facebook      *string `json:"facebook"`
	LineId        *string `json:"lineId"`
	ShowInstagram bool    `json:"showInstagram"`
	ShowFacebook  bool    `json:"showFacebook"`
	ShowLine      bool    `json:"showLine"`
}

// ======== Store + Persistence ========

type Store struct {
	mu sync.RWMutex

	// 帖文
	posts []Post

	// 追蹤標籤：userId -> tags (unique, lowercased for equality)
	tags map[string][]string

	// 好友/追蹤：userId -> set(friendId)
	friends map[string]map[string]struct{}

	// 使用者公開資料
	profiles map[string]Profile
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

func ensureDir(d string) { _ = os.MkdirAll(d, 0o755) }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ----- File helpers -----

func readJSONFile[T any](path string, out *T) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (s *Store) loadAll(postsFile, tagsFile, friendsFile, profilesFile string) {
	// posts
	_ = readJSONFile(postsFile, &s.posts)
	if s.tags == nil {
		s.tags = make(map[string][]string)
	}
	if s.friends == nil {
		s.friends = make(map[string]map[string]struct{})
	}
	if s.profiles == nil {
		s.profiles = make(map[string]Profile)
	}
	_ = readJSONFile(tagsFile, &s.tags)
	_ = readJSONFile(friendsFile, &s.friends)
	_ = readJSONFile(profilesFile, &s.profiles)
}

func (s *Store) savePosts(path string)    { _ = writeJSONFile(path, s.posts) }
func (s *Store) saveTags(path string)     { _ = writeJSONFile(path, s.tags) }
func (s *Store) saveFriends(path string)  { _ = writeJSONFile(path, s.friends) }
func (s *Store) saveProfiles(path string) { _ = writeJSONFile(path, s.profiles) }

// ======== Posts in-memory ops ========

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

func (s *Store) userPosts(uid string) []Post {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Post
	for _, p := range s.posts {
		if p.Author.ID == uid {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// ======== Tags helpers ========

func normalizeTag(t string) string {
	return strings.TrimSpace(strings.ToLower(t))
}

func (s *Store) getTags(uid string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.tags[uid]...)
}

func (s *Store) addTag(uid, tag string) []string {
	t := normalizeTag(tag)
	if t == "" {
		return s.getTags(uid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.tags[uid]
	for _, x := range cur {
		if x == t {
			return append([]string(nil), cur...)
		}
	}
	cur = append(cur, t)
	s.tags[uid] = cur
	return append([]string(nil), cur...)
}

func (s *Store) removeTag(uid, tag string) []string {
	t := normalizeTag(tag)
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.tags[uid]
	var out []string
	for _, x := range cur {
		if x != t {
			out = append(out, x)
		}
	}
	s.tags[uid] = out
	return append([]string(nil), out...)
}

// ======== Friends helpers ========

func (s *Store) getFriends(uid string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	set := s.friends[uid]
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *Store) follow(uid, target string) {
	if uid == "" || target == "" || uid == target {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.friends[uid]
	if m == nil {
		m = make(map[string]struct{})
		s.friends[uid] = m
	}
	m[target] = struct{}{}
}

func (s *Store) unfollow(uid, target string) {
	if uid == "" || target == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.friends[uid]
	if m == nil {
		return
	}
	delete(m, target)
}

// ======== Profiles helpers ========

func (s *Store) getProfile(uid string) (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[uid]
	return p, ok
}

func (s *Store) upsertProfile(p Profile) Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		return p
	}
	// 合併更新（僅覆蓋非零值）
	ex, ok := s.profiles[p.ID]
	if !ok {
		s.profiles[p.ID] = p
		return p
	}
	if p.Name != "" {
		ex.Name = p.Name
	}
	if p.Nickname != nil {
		ex.Nickname = p.Nickname
	}
	if p.AvatarURL != nil {
		ex.AvatarURL = p.AvatarURL
	}
	if p.Instagram != nil {
		ex.Instagram = p.Instagram
	}
	if p.Facebook != nil {
		ex.Facebook = p.Facebook
	}
	if p.LineId != nil {
		ex.LineId = p.LineId
	}
	// bool 有值時才有意義（零值 false 也可能是刻意）
	ex.ShowInstagram = p.ShowInstagram
	ex.ShowFacebook = p.ShowFacebook
	ex.ShowLine = p.ShowLine

	s.profiles[p.ID] = ex
	return ex
}

// ======== main ========

func main() {
	// 資料目錄
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}
	uploadsDir := filepath.Join(dataDir, "uploads")
	postsFile := filepath.Join(dataDir, "posts.json")
	tagsFile := filepath.Join(dataDir, "tags.json")
	friendsFile := filepath.Join(dataDir, "friends.json")
	profilesFile := filepath.Join(dataDir, "profiles.json")
	ensureDir(uploadsDir)

	store := &Store{
		tags:     map[string][]string{},
		friends:  map[string]map[string]struct{}{},
		profiles: map[string]Profile{},
	}
	store.loadAll(postsFile, tagsFile, friendsFile, profilesFile)

	// 首次啟動塞一些 demo 資料
	func() {
		store.mu.RLock()
		emptyPosts := len(store.posts) == 0
		_, hasMe := store.profiles["u_me"]
		_, hasBob := store.profiles["u_bob"]
		store.mu.RUnlock()

		if emptyPosts {
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
			store.savePosts(postsFile)
		}
		if !hasMe {
			nick := "Me"
			store.upsertProfile(Profile{ID: "u_me", Name: "Me", Nickname: &nick})
		}
		if !hasBob {
			nick := "Bob"
			insta := "@bob_dev"
			store.upsertProfile(Profile{
				ID:            "u_bob",
				Name:          "Bob",
				Nickname:      &nick,
				Instagram:     &insta,
				ShowInstagram: true,
			})
		}
	}()

	mux := http.NewServeMux()

	// 健康檢查
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// CORS（Flutter Web 也可用）
	cors := func(next http.Handler) http.Handler {
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

	// ---- 靜態檔案：/uploads/*
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadsDir))))

	// ---- 圖片上傳：POST /upload
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
		dst := filepath.Join(uploadsDir, filename)

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
	})

	// ---- /posts：GET 列表、POST 建立
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
				ID:        time.Now().Format("20060102T150405.000000000"),
				Author:    req.Author,
				Text:      req.Text,
				CreatedAt: nowISO(),
				LikeCount: 0,
				Comments:  []Comment{},
				Tags:      req.Tags,
				ImageURL:  req.ImageURL,
			}
			created := store.create(p)
			store.savePosts(postsFile)
			writeJSON(w, http.StatusOK, created)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// ---- /posts/{id}, /posts/{id}/like, /posts/{id}/comments
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
				updated := store.updateAt(idx, p)
				store.savePosts(postsFile)
				writeJSON(w, http.StatusOK, updated)

			case http.MethodDelete:
				p, idx := store.byID(id)
				if idx < 0 {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				if p.ImageURL != nil && strings.HasPrefix(*p.ImageURL, "/uploads/") {
					_ = os.Remove(filepath.Join(uploadsDir, filepath.Base(*p.ImageURL)))
				}
				store.deleteAt(idx)
				store.savePosts(postsFile)
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
			updated := store.updateAt(idx, p)
			store.savePosts(postsFile)
			writeJSON(w, http.StatusOK, updated)

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
			p.Comments = append(p.Comments, Comment{
				ID:        time.Now().Format("20060102T150405.000000000"),
				Author:    req.Author,
				Text:      req.Text,
				CreatedAt: nowISO(),
			})
			updated := store.updateAt(idx, p)
			store.savePosts(postsFile)
			writeJSON(w, http.StatusOK, updated)

		default:
			http.NotFound(w, r)
		}
	})

	// ---- /me：GET 讀自己的 Profile、PATCH 更新
	//   透過 ?uid= 指定目前使用者（沒帶就用 "u_me"）
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		uid := r.URL.Query().Get("uid")
		if uid == "" {
			uid = "u_me"
		}
		switch r.Method {
		case http.MethodGet:
			if p, ok := store.getProfile(uid); ok {
				writeJSON(w, http.StatusOK, p)
				return
			}
			http.Error(w, "not found", http.StatusNotFound)
		case http.MethodPatch:
			var p Profile
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// 忽略 body.id 若與 uid 不同，強制用 uid
			p.ID = uid
			updated := store.upsertProfile(p)
			store.saveProfiles(profilesFile)
			writeJSON(w, http.StatusOK, updated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// ---- /me/tags：GET 取列表、POST 新增、DELETE /me/tags/{tag} 移除
	mux.HandleFunc("/me/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/tags" {
			http.NotFound(w, r)
			return
		}
		uid := r.URL.Query().Get("uid")
		if uid == "" {
			uid = "u_me"
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, store.getTags(uid))
		case http.MethodPost:
			var body struct {
				Tag string `json:"tag"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			tags := store.addTag(uid, body.Tag)
			store.saveTags(tagsFile)
			writeJSON(w, http.StatusOK, tags)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/me/tags/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		uid := r.URL.Query().Get("uid")
		if uid == "" {
			uid = "u_me"
		}
		tag := strings.TrimPrefix(r.URL.Path, "/me/tags/")
		if tag == "" {
			http.NotFound(w, r)
			return
		}
		tags := store.removeTag(uid, tag)
		store.saveTags(tagsFile)
		writeJSON(w, http.StatusOK, tags)
	})

	// ---- /me/friends：GET 我的好友 ID 列表
	mux.HandleFunc("/me/friends", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		uid := r.URL.Query().Get("uid")
		if uid == "" {
			uid = "u_me"
		}
		writeJSON(w, http.StatusOK, store.getFriends(uid))
	})

	// ---- /users/{id}、/users/{id}/follow、/users/{id}/posts
	mux.HandleFunc("/users/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/users/")
		if rest == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(rest, "/")
		userId := parts[0]

		if len(parts) == 1 {
			// GET /users/{id}
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if p, ok := store.getProfile(userId); ok {
				writeJSON(w, http.StatusOK, p)
				return
			}
			// 若沒有 profile，回傳最基本資訊
			name := userId
			writeJSON(w, http.StatusOK, Profile{ID: userId, Name: name})
			return
		}

		switch parts[1] {
		case "follow":
			uid := r.URL.Query().Get("uid")
			if uid == "" {
				uid = "u_me"
			}
			switch r.Method {
			case http.MethodPost:
				store.follow(uid, userId)
				store.saveFriends(friendsFile)
				w.WriteHeader(http.StatusNoContent)
			case http.MethodDelete:
				store.unfollow(uid, userId)
				store.saveFriends(friendsFile)
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		case "posts":
			// GET /users/{id}/posts
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, store.userPosts(userId))
		default:
			http.NotFound(w, r)
		}
	})

	// ---- Start server ----
	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}
	addr := ":" + port
	log.Println("Server listening on", addr, "DATA_DIR=", dataDir)
	if err := http.ListenAndServe(addr, cors(mux)); err != nil {
		log.Fatal(err)
	}
}
