package httpx

import (
	"encoding/json"
	"net/http"
	"strings"

	"local.dev/socialdemo-backend/internal/models"
)

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
