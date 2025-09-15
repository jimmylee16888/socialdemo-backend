package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"local.dev/socialdemo-backend/internal/models"
)

type Store struct {
	mu        sync.RWMutex
	posts     []models.Post
	tags      map[string][]string            // userId -> tags
	friends   map[string]map[string]struct{} // userId -> set(friendId)
	profiles  map[string]models.Profile      // userId -> profile
	postLikes map[string]map[string]struct{} // postId -> set(uid)
}

func NewStore() *Store {
	return &Store{
		tags:      map[string][]string{},
		friends:   map[string]map[string]struct{}{},
		profiles:  map[string]models.Profile{},
		postLikes: map[string]map[string]struct{}{},
	}
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

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
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	return os.WriteFile(path, b, 0o644)
}

func (s *Store) LoadAll(posts, tags, friends, profiles, likes string) {
	_ = readJSONFile(posts, &s.posts)
	if s.tags == nil {
		s.tags = make(map[string][]string)
	}
	if s.friends == nil {
		s.friends = make(map[string]map[string]struct{})
	}
	if s.profiles == nil {
		s.profiles = make(map[string]models.Profile)
	}
	if s.postLikes == nil {
		s.postLikes = make(map[string]map[string]struct{})
	}
	_ = readJSONFile(tags, &s.tags)
	_ = readJSONFile(friends, &s.friends)
	_ = readJSONFile(profiles, &s.profiles)
	_ = readJSONFile(likes, &s.postLikes)
}

func (s *Store) SavePosts(path string)    { _ = writeJSONFile(path, s.posts) }
func (s *Store) SaveTags(path string)     { _ = writeJSONFile(path, s.tags) }
func (s *Store) SaveFriends(path string)  { _ = writeJSONFile(path, s.friends) }
func (s *Store) SaveProfiles(path string) { _ = writeJSONFile(path, s.profiles) }
func (s *Store) SaveLikes(path string)    { _ = writeJSONFile(path, s.postLikes) }

// Demo seed
func (s *Store) SeedIfEmpty(postsFile string) {
	s.mu.RLock()
	empty := len(s.posts) == 0
	_, hasMe := s.profiles["u_me"]
	_, hasBob := s.profiles["u_bob"]
	s.mu.RUnlock()

	if empty {
		s.Create(models.Post{
			ID:        "p1",
			Author:    models.User{ID: "u_bob", Name: "Bob"},
			Text:      "今天把 UI 卡片邊角修好了 ✅",
			CreatedAt: nowISO(),
			Comments:  []models.Comment{},
			Tags:      []string{"flutter", "design"},
		})
		s.Create(models.Post{
			ID:        "p2",
			Author:    models.User{ID: "u_me", Name: "Me"},
			Text:      "嗨！這是我的第一篇 🙂",
			CreatedAt: nowISO(),
			Comments:  []models.Comment{},
			Tags:      []string{"hello"},
		})
		s.SavePosts(postsFile)
	}
	if !hasMe {
		nick := "Me"
		s.UpsertProfile(models.Profile{ID: "u_me", Name: "Me", Nickname: &nick})
	}
	if !hasBob {
		nick := "Bob"
		insta := "@bob_dev"
		s.UpsertProfile(models.Profile{
			ID:            "u_bob",
			Name:          "Bob",
			Nickname:      &nick,
			Instagram:     &insta,
			ShowInstagram: true,
		})
	}
}

// ===== 公開：顯示名稱 / 裝飾（計算 LikeCount / LikedByMe） =====

func (s *Store) DisplayName(uid string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.profiles[uid]; ok {
		if p.Nickname != nil && *p.Nickname != "" {
			return *p.Nickname
		}
		if p.Name != "" {
			return p.Name
		}
	}
	return uid
}

func (s *Store) Decorate(p models.Post, viewerUID string) models.Post {
	cp := p
	if cp.Author.ID != "" {
		cp.Author.Name = s.DisplayName(cp.Author.ID)
	}
	s.mu.RLock()
	set := s.postLikes[cp.ID]
	s.mu.RUnlock()
	cp.LikeCount = len(set)
	_, liked := set[viewerUID]
	cp.LikedByMe = liked
	return cp
}

// ===== 列表 / CRUD =====

func (s *Store) List(tab string, tags []string, viewerUID string) []models.Post {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var base []models.Post
	if len(tags) > 0 {
		tagset := map[string]struct{}{}
		for _, t := range tags {
			tagset[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
		}
		for _, p := range s.posts {
			for _, pt := range p.Tags {
				if _, ok := tagset[strings.ToLower(pt)]; ok {
					base = append(base, p)
					break
				}
			}
		}
	} else {
		base = append(base, s.posts...)
	}

	out := make([]models.Post, 0, len(base))
	for _, p := range base {
		out = append(out, s.Decorate(p, viewerUID))
	}

	if tab == "hot" {
		sort.Slice(out, func(i, j int) bool {
			if out[i].LikeCount == out[j].LikeCount {
				return out[i].CreatedAt > out[j].CreatedAt
			}
			return out[i].LikeCount > out[j].LikeCount
		})
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	}
	return out
}

func (s *Store) Create(p models.Post) models.Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.posts = append([]models.Post{p}, s.posts...)
	return p
}

func (s *Store) ByID(id string) (models.Post, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i, p := range s.posts {
		if p.ID == id {
			return p, i
		}
	}
	return models.Post{}, -1
}

func (s *Store) UpdateAt(i int, p models.Post) models.Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.posts[i] = p
	return p
}

func (s *Store) DeleteAt(i int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.posts = append(s.posts[:i], s.posts[i+1:]...)
}

func (s *Store) UserPosts(uid, viewerUID string) []models.Post {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []models.Post
	for _, p := range s.posts {
		if p.Author.ID == uid {
			out = append(out, s.Decorate(p, viewerUID))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// ===== tags =====

func normalizeTag(t string) string { return strings.TrimSpace(strings.ToLower(t)) }

func (s *Store) GetTags(uid string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.tags[uid]...)
}

func (s *Store) AddTag(uid, tag string) []string {
	t := normalizeTag(tag)
	if t == "" {
		return s.GetTags(uid)
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

func (s *Store) RemoveTag(uid, tag string) []string {
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

// ===== friends =====

func (s *Store) GetFriends(uid string) []string {
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

func (s *Store) Follow(uid, target string) {
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

func (s *Store) Unfollow(uid, target string) {
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

// ===== likes =====

func (s *Store) ToggleLike(postID, uid string) (models.Post, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, p := range s.posts {
		if p.ID == postID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return models.Post{}, false
	}
	p := s.posts[idx]
	set := s.postLikes[p.ID]
	if set == nil {
		set = make(map[string]struct{})
	}
	if _, ok := set[uid]; ok {
		delete(set, uid)
	} else {
		set[uid] = struct{}{}
	}
	s.postLikes[p.ID] = set
	p.LikeCount = len(set)
	_, liked := set[uid]
	p.LikedByMe = liked
	s.posts[idx] = p
	return p, true
}
