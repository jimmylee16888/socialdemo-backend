package models

type User struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	AvatarAsset *string `json:"avatarAsset,omitempty"`
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
