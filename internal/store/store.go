package store

import (
	"encoding/json"
	"fmt"
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
	profiles  map[string]models.Profile      // userId -> profile (定義在 profile.go 的 Get/Upsert 使用)
	postLikes map[string]map[string]struct{} // postId -> set(uid)

	// 🔻 新增
	boards        map[string]models.Board
	conversations map[string]models.Conversation
	messages      map[string]models.Message
}

func NewStore() *Store {
	return &Store{
		tags:      map[string][]string{},
		friends:   map[string]map[string]struct{}{},
		profiles:  map[string]models.Profile{},
		postLikes: map[string]map[string]struct{}{},

		// 🔻 新增
		boards:        map[string]models.Board{},
		conversations: map[string]models.Conversation{},
		messages:      map[string]models.Message{},
	}
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

// 共用 ID 產生器（Boards / Conversations / Messages 都可以用）
func newID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}

// 解析 ISO 時間字串；失敗時回傳零值
func parseISO(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

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

func (s *Store) LoadBoards(path string) {
	if s.boards == nil {
		s.boards = make(map[string]models.Board)
	}
	_ = readJSONFile(path, &s.boards)
}

func (s *Store) LoadDM(conversationsPath, messagesPath string) {
	if s.conversations == nil {
		s.conversations = make(map[string]models.Conversation)
	}
	if s.messages == nil {
		s.messages = make(map[string]models.Message)
	}
	_ = readJSONFile(conversationsPath, &s.conversations)
	_ = readJSONFile(messagesPath, &s.messages)
}

func (s *Store) SaveBoards(path string)        { _ = writeJSONFile(path, s.boards) }
func (s *Store) SaveConversations(path string) { _ = writeJSONFile(path, s.conversations) }
func (s *Store) SaveMessages(path string)      { _ = writeJSONFile(path, s.messages) }
func (s *Store) SavePosts(path string)         { _ = writeJSONFile(path, s.posts) }
func (s *Store) SaveTags(path string)          { _ = writeJSONFile(path, s.tags) }
func (s *Store) SaveFriends(path string)       { _ = writeJSONFile(path, s.friends) }
func (s *Store) SaveProfiles(path string)      { _ = writeJSONFile(path, s.profiles) }
func (s *Store) SaveLikes(path string)         { _ = writeJSONFile(path, s.postLikes) }

// Demo seed
// Demo seed
func (s *Store) SeedIfEmpty(postsFile string) {
	s.mu.RLock()
	empty := len(s.posts) == 0
	_, hasAlice := s.profiles["demo_alice"]
	_, hasBob := s.profiles["demo_bob"]
	s.mu.RUnlock()

	if empty {
		seed := []models.Post{
			{
				ID:        "p1",
				Author:    models.User{ID: "demo_bob", Name: "Bob"},
				Text:      "今天把動態牆的 UI 卡片邊角修好了 ✅ 現在拿自己的應援小卡來排版超漂亮～",
				CreatedAt: nowISO(),
				Comments:  []models.Comment{},
				Tags:      []string{"flutter", "design", "devlog"},
			},
			{
				ID:        "p2",
				Author:    models.User{ID: "demo_alice", Name: "Alice"},
				Text:      "嗨！這是我的第一篇 🙂 以後想在這裡紀錄我的 K-pop 小卡收藏！",
				CreatedAt: nowISO(),
				Comments:  []models.Comment{},
				Tags:      []string{"hello", "kpop", "photocard"},
			},
			{
				ID:        "p3",
				Author:    models.User{ID: "demo_alice", Name: "Alice"},
				Text:      "今天把 LE SSERAFIM 新專的小卡都輸入進 APP 了 🃏\n感覺自己的「偶像空間」慢慢成形，好有成就感！",
				CreatedAt: nowISO(),
				Comments:  []models.Comment{},
				Tags:      []string{"kpop", "lesserafim", "collection", "idol-room"},
			},
			{
				ID:        "p4",
				Author:    models.User{ID: "demo_bob", Name: "Bob"},
				Text:      "有沒有人想換小卡？我這裡多了好幾張重複的 🥲\n之後想做一個『交換中』的專區，讓大家更好配對。",
				CreatedAt: nowISO(),
				Comments:  []models.Comment{},
				Tags:      []string{"trade", "photocard", "feature-idea"},
			},
			{
				ID:        "p5",
				Author:    models.User{ID: "demo_alice", Name: "Alice"},
				Text:      "剛把專輯架上的封面照都拍起來放進 APP 的專輯牆 📀\n滑一滑真的很像在逛自己的小型展覽館。",
				CreatedAt: nowISO(),
				Comments:  []models.Comment{},
				Tags:      []string{"album", "shelf", "collection", "design"},
			},
			{
				ID:        "p6",
				Author:    models.User{ID: "demo_bob", Name: "Bob"},
				Text:      "想做一個『我的偶像空間』主題頁：\n背景可以放舞台照，前面是小卡、專輯、應援棒一起排版，\n再加上動態貼文，就變成專屬自己的 idol profile ✨",
				CreatedAt: nowISO(),
				Comments:  []models.Comment{},
				Tags:      []string{"idea", "idol-space", "kpop", "ui"},
			},
		}

		for _, p := range seed {
			s.Create(p)
		}
		s.SavePosts(postsFile)
	}

	// Profile 的 Upsert / Get 在 profile.go，這裡只呼叫
	if !hasAlice {
		nick := "Alice"
		s.UpsertProfile(models.Profile{
			ID:       "demo_alice",
			Name:     "Alice",
			Nickname: &nick,
		})
	}
	if !hasBob {
		nick := "Bob"
		insta := "@bob_dev"
		s.UpsertProfile(models.Profile{
			ID:            "demo_bob",
			Name:          "Bob",
			Nickname:      &nick,
			Instagram:     &insta,
			ShowInstagram: true,
		})
	}
}

// ===== 顯示名稱（由 Profile 統一） + 裝飾（LikeCount/LikedByMe、留言作者名也一致）=====

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

	// 作者顯示名
	if cp.Author.ID != "" {
		cp.Author.Name = s.DisplayName(cp.Author.ID)
	}

	// 留言作者顯示名一致化
	for i := range cp.Comments {
		if cp.Comments[i].Author.ID != "" {
			cp.Comments[i].Author.Name = s.DisplayName(cp.Comments[i].Author.ID)
		}
	}

	// Like 累計 / 是否由我按讚
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

// 依作者清單與(可選)標籤過濾貼文，並套用 Decorate；結果依時間新→舊。
// 依作者清單 + (可選) 標籤 過濾，並 Decorate + 依時間排序（或照 hot 需求改）
// store/store.go
func (s *Store) ListByAuthors(authors []string, tags []string, viewerUID string) []models.Post {
	s.mu.RLock()
	defer s.mu.RUnlock()

	authorSet := map[string]struct{}{}
	for _, a := range authors {
		a = strings.TrimSpace(a)
		if a != "" {
			authorSet[a] = struct{}{}
		}
	}

	tagSet := map[string]struct{}{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			tagSet[t] = struct{}{}
		}
	}

	// ✅ 用空 slice，而不是 nil
	out := make([]models.Post, 0)

	for _, p := range s.posts {
		if _, ok := authorSet[p.Author.ID]; !ok {
			continue
		}
		if len(tagSet) > 0 {
			match := false
			for _, pt := range p.Tags {
				if _, ok := tagSet[strings.ToLower(pt)]; ok {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, s.Decorate(p, viewerUID))
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// 依 boardId + (可選) tags 篩選貼文，並 Decorate 後依時間排序新→舊
func (s *Store) ListByBoard(boardID string, tags []string, viewerUID string) []models.Post {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if boardID == "" {
		return nil
	}

	tagSet := map[string]struct{}{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			tagSet[t] = struct{}{}
		}
	}

	out := make([]models.Post, 0)
	for _, p := range s.posts {
		if p.BoardID != boardID {
			continue
		}
		if len(tagSet) > 0 {
			match := false
			for _, pt := range p.Tags {
				if _, ok := tagSet[strings.ToLower(pt)]; ok {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, s.Decorate(p, viewerUID))
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// ===== Boards =====

// 列出某使用者可以看到的所有 boards（排除 deleted / 私人但不是 owner 的）
func (s *Store) ListBoardsFor(uid string) []models.Board {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]models.Board, 0, len(s.boards))
	for _, b := range s.boards {
		if b.Deleted {
			continue
		}
		if b.IsPrivate && b.OwnerID != uid {
			continue
		}
		out = append(out, b)
	}

	// 依 createdAt 新 → 舊
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})

	return out
}

func (s *Store) GetBoard(id string) (models.Board, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.boards[id]
	return b, ok
}

func (s *Store) SaveBoard(b models.Board) models.Board {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.boards == nil {
		s.boards = make(map[string]models.Board)
	}

	// ⭐ 這裡補 ID
	if b.ID == "" {
		b.ID = newID("b")
	}

	s.boards[b.ID] = b
	return b
}

// ===== DM (Conversations & Messages) =====

func (s *Store) ListConversationsFor(uid string) []models.Conversation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]models.Conversation, 0, len(s.conversations))
	for _, c := range s.conversations {
		if containsString(c.MemberIDs, uid) {
			out = append(out, c)
		}
	}

	// 依 lastMessageAt / createdAt 新 → 舊
	sort.Slice(out, func(i, j int) bool {
		ti := parseISO(out[i].LastMessageAt)
		if ti.IsZero() {
			ti = parseISO(out[i].CreatedAt)
		}
		tj := parseISO(out[j].LastMessageAt)
		if tj.IsZero() {
			tj = parseISO(out[j].CreatedAt)
		}
		return ti.After(tj)
	})

	return out
}

func (s *Store) GetConversation(id string) (models.Conversation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.conversations[id]
	return c, ok
}

func (s *Store) SaveConversation(c models.Conversation) models.Conversation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conversations == nil {
		s.conversations = make(map[string]models.Conversation)
	}

	// ⭐ 這裡補 ID
	if c.ID == "" {
		c.ID = newID("c")
	}

	s.conversations[c.ID] = c
	return c
}

func (s *Store) ListMessages(convID string, after, before time.Time, limit int) []models.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs := make([]models.Message, 0)
	for _, m := range s.messages {
		if m.ConversationID != convID || m.Deleted {
			continue
		}
		mt := parseISO(m.CreatedAt)
		if !after.IsZero() && !mt.After(after) {
			continue
		}
		if !before.IsZero() && !mt.Before(before) {
			continue
		}
		msgs = append(msgs, m)
	}

	sort.Slice(msgs, func(i, j int) bool {
		return parseISO(msgs[i].CreatedAt).Before(parseISO(msgs[j].CreatedAt))
	})
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return msgs
}

func (s *Store) SaveMessage(m models.Message) models.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messages == nil {
		s.messages = make(map[string]models.Message)
	}
	// 如果外面沒給 ID，就自己生一個
	if m.ID == "" {
		m.ID = newID("m")
	}
	s.messages[m.ID] = m

	// 更新 conversation 的 lastMessageAt / preview
	if c, ok := s.conversations[m.ConversationID]; ok {
		c.LastMessageAt = m.CreatedAt
		if m.Text != "" {
			c.LastMessagePreview = m.Text
		} else {
			switch m.Type {
			case "miniCard":
				c.LastMessagePreview = "[Mini Card]"
			case "album":
				c.LastMessagePreview = "[Album]"
			default:
				c.LastMessagePreview = ""
			}
		}
		s.conversations[c.ID] = c
	}

	return m
}

func (s *Store) LoadAll(postsFile, tagsFile, friendsFile, profilesFile, likesFile string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// posts
	_ = readJSONFile(postsFile, &s.posts)

	// tags
	if s.tags == nil {
		s.tags = make(map[string][]string)
	}
	_ = readJSONFile(tagsFile, &s.tags)

	// friends
	if s.friends == nil {
		s.friends = make(map[string]map[string]struct{})
	}
	_ = readJSONFile(friendsFile, &s.friends)

	// profiles
	if s.profiles == nil {
		s.profiles = make(map[string]models.Profile)
	}
	_ = readJSONFile(profilesFile, &s.profiles)

	// likes
	if s.postLikes == nil {
		s.postLikes = make(map[string]map[string]struct{})
	}
	_ = readJSONFile(likesFile, &s.postLikes)
}
