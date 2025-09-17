package httpx

import (
	"net/http"
	"strings"

	"local.dev/socialdemo-backend/internal/models"
)

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
