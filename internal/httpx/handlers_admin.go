package httpx

import "net/http"

func HandleAdminReload(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		app.Store.LoadAll(app.Paths.PostsFile, app.Paths.TagsFile,
			app.Paths.FriendsFile, app.Paths.ProfilesFile, app.Paths.LikesFile)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
