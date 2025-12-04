package httpx

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"local.dev/socialdemo-backend/internal/models"
)

// GET /conversations ；POST /conversations
func HandleConversations(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUID(r)

		switch r.Method {
		case http.MethodGet:
			convs := app.Store.ListConversationsFor(uid)
			// unreadCount 目前沒有做，就讓前端自己預設 0
			writeJSON(w, http.StatusOK, convs)

		case http.MethodPost:
			var in struct {
				MemberIDs []string `json:"memberIds"`
				Name      string   `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			if len(in.MemberIDs) == 0 {
				in.MemberIDs = []string{uid}
			}
			// 確保自己在裡面
			foundMe := false
			for _, id := range in.MemberIDs {
				if strings.TrimSpace(id) == uid {
					foundMe = true
					break
				}
			}
			if !foundMe {
				in.MemberIDs = append(in.MemberIDs, uid)
			}

			now := time.Now().UTC().Format(time.RFC3339)

			// 去除空白與重複成員
			memberSet := map[string]struct{}{}
			members := make([]string, 0, len(in.MemberIDs))
			for _, m := range in.MemberIDs {
				m = strings.TrimSpace(m)
				if m == "" {
					continue
				}
				if _, ok := memberSet[m]; ok {
					continue
				}
				memberSet[m] = struct{}{}
				members = append(members, m)
			}

			c := models.Conversation{
				// ⭐ 這裡 ID 先給空字串
				ID:                 "",
				Name:               strings.TrimSpace(in.Name),
				MemberIDs:          members,
				CreatedAt:          now,
				LastMessageAt:      "",
				LastMessagePreview: "",
			}

			// ⭐ 交給 Store 補 ID
			c = app.Store.SaveConversation(c)
			app.Store.SaveConversations(app.Paths.ConversationsFile)

			writeJSON(w, http.StatusCreated, c)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

// GET /conversations/{id}/messages
// POST /conversations/{id}/messages
func HandleConversationMessages(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUID(r)
		path := strings.TrimPrefix(r.URL.Path, "/conversations/")
		if path == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(path, "/")
		if len(parts) != 2 || parts[1] != "messages" {
			http.NotFound(w, r)
			return
		}
		convID := parts[0]

		switch r.Method {
		case http.MethodGet:
			handleFetchMessages(app, w, r, uid, convID)
		case http.MethodPost:
			handleSendMessage(app, w, r, uid, convID)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func handleFetchMessages(app *AppCtx, w http.ResponseWriter, r *http.Request, uid, convID string) {
	q := r.URL.Query()
	afterStr := q.Get("after")
	beforeStr := q.Get("before")
	limitStr := q.Get("limit")

	var after, before time.Time
	if afterStr != "" {
		if t, err := time.Parse(time.RFC3339, afterStr); err == nil {
			after = t.UTC()
		}
	}
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

	// 確認會議存在且自己在成員裡
	conv, ok := app.Store.GetConversation(convID)
	if !ok || !containsString(conv.MemberIDs, uid) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
		return
	}

	msgs := app.Store.ListMessages(convID, after, before, limit)
	writeJSON(w, http.StatusOK, msgs)
}

func handleSendMessage(app *AppCtx, w http.ResponseWriter, r *http.Request, uid, convID string) {
	var in struct {
		Type          string         `json:"type"`
		Text          string         `json:"text"`
		ContentJSON   map[string]any `json:"contentJson"`
		ContentSchema string         `json:"contentSchema"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if in.Type == "" {
		in.Type = "text"
	}

	conv, ok := app.Store.GetConversation(convID)
	if !ok || !containsString(conv.MemberIDs, uid) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	m := models.Message{
		ID:             "", // ⭐ 讓 Store 自己 newID("m")
		ConversationID: convID,
		SenderID:       uid,
		Type:           in.Type,
		Text:           in.Text,
		ContentSchema:  in.ContentSchema,
		ContentJson:    in.ContentJSON,
		CreatedAt:      now,
		Deleted:        false,
	}

	m = app.Store.SaveMessage(m)
	app.Store.SaveMessages(app.Paths.MessagesFile)
	app.Store.SaveConversations(app.Paths.ConversationsFile)

	writeJSON(w, http.StatusCreated, m)

}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
