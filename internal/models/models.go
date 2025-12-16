package models

type User struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
}

type Comment struct {
	ID        string `json:"id"`
	Author    User   `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"` // ISO 8601
}

type Post struct {
	ID        string    `json:"id"`
	Author    User      `json:"author"`
	Text      string    `json:"text"`
	CreatedAt string    `json:"createdAt"` // ISO 8601
	LikeCount int       `json:"likeCount"`
	LikedByMe bool      `json:"likedByMe"`
	Comments  []Comment `json:"comments"`
	Tags      []string  `json:"tags"`
	ImageURL  *string   `json:"imageUrl,omitempty"` // e.g. "/uploads/xxx.jpg"

	// 🔻 新增：貼文所屬 board（可空）
	BoardID string `json:"boardId,omitempty"`
}

type Profile struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Nickname      *string `json:"nickname"`
	AvatarURL     *string `json:"avatarUrl"`
	Instagram     *string `json:"instagram"`
	Facebook      *string `json:"facebook"`
	LineId        *string `json:"lineId"`
	Birthday      string  `json:"birthday,omitempty"` // 格式 yyyy-MM-dd
	ShowInstagram bool    `json:"showInstagram"`
	ShowFacebook  bool    `json:"showFacebook"`
	ShowLine      bool    `json:"showLine"`
}

type Board struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	OwnerID      string   `json:"ownerId"`
	ModeratorIDs []string `json:"moderatorIds,omitempty"`
	IsOfficial   bool     `json:"isOfficial,omitempty"`
	IsPrivate    bool     `json:"isPrivate,omitempty"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
	Deleted      bool     `json:"deleted,omitempty"`
}

type Conversation struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name,omitempty"`
	MemberIDs          []string `json:"memberIds"`
	CreatedAt          string   `json:"createdAt"`
	LastMessageAt      string   `json:"lastMessageAt,omitempty"`
	LastMessagePreview string   `json:"lastMessagePreview,omitempty"`
}

type Message struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversationId"`
	SenderID       string         `json:"senderId"`
	Type           string         `json:"type"` // "text" / "miniCard" / "album"
	Text           string         `json:"text,omitempty"`
	ContentSchema  string         `json:"contentSchema,omitempty"`
	ContentJson    map[string]any `json:"contentJson,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	Deleted        bool           `json:"deleted,omitempty"`
}
