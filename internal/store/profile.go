package store

import (
	"local.dev/socialdemo-backend/internal/models"
)

// 讀取單一使用者 Profile
func (s *Store) GetProfile(uid string) (models.Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[uid]
	return p, ok
}

// 新增或更新 Profile（僅覆蓋有提供的欄位）
func (s *Store) UpsertProfile(p models.Profile) models.Profile {
	if p.ID == "" {
		return p
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ex, ok := s.profiles[p.ID]
	if !ok {
		// 新增
		s.profiles[p.ID] = p
		return p
	}

	// 更新：僅覆蓋有提供的欄位
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
	// 這三個是 boolean（非指標），直接覆蓋
	ex.ShowInstagram = p.ShowInstagram
	ex.ShowFacebook = p.ShowFacebook
	ex.ShowLine = p.ShowLine

	s.profiles[p.ID] = ex
	return ex
}
