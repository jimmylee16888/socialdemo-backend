package main

import (
	"log"
	"net/http"
	"os"

	"local.dev/socialdemo-backend/internal/config"
	"local.dev/socialdemo-backend/internal/httpx"
	"local.dev/socialdemo-backend/internal/store"
)

func main() {
	// 檔案路徑與資料夾
	cfg := config.DefaultPaths()
	config.EnsureDir(cfg.DataDir)
	config.EnsureDir(cfg.UploadsDir)

	// 資料層（本地 JSON 持久化）
	st := store.NewStore()
	st.LoadAll(cfg.PostsFile, cfg.TagsFile, cfg.FriendsFile, cfg.ProfilesFile, cfg.LikesFile)
	st.SeedIfEmpty(cfg.PostsFile) // 第一次啟動塞 demo

	// Firebase（驗證保留；NO_AUTH=1 時走免驗證）
	authClient := config.NewAuthClient()

	app := &httpx.AppCtx{
		Store:      st,
		AuthClient: authClient,
		Paths:      cfg,
	}

	// 路由
	mux := http.NewServeMux()

	// 管理介面
	mux.Handle("/admin/", http.StripPrefix("/admin/", http.FileServer(http.Dir("web/admin"))))
	mux.HandleFunc("/admin/reload", httpx.WithAuth(app, httpx.HandleAdminReload(app)))

	// 健康檢查
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// 靜態檔 (上傳目錄)
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(cfg.UploadsDir))))

	// 上傳
	mux.HandleFunc("/upload", httpx.WithAuth(app, httpx.HandleUpload(app)))

	// 貼文
	mux.HandleFunc("/posts", httpx.HandlePosts(app))       // GET/POST
	mux.HandleFunc("/posts/", httpx.HandlePostDetail(app)) // PUT/DELETE、/like、/comments

	// （新增）依朋友清單查貼文
	mux.HandleFunc("/posts/query", httpx.WithAuth(app, httpx.HandlePostsQuery(app))) // POST

	// 自己 Profile
	mux.HandleFunc("/me", httpx.WithAuth(app, httpx.HandleMe(app))) // GET/PATCH

	// 自己 tags、friends
	mux.HandleFunc("/me/tags", httpx.WithAuth(app, httpx.HandleMyTags(app)))        // GET/POST
	mux.HandleFunc("/me/tags/", httpx.WithAuth(app, httpx.HandleMyTagsDelete(app))) // DELETE /me/tags/{tag}
	mux.HandleFunc("/me/friends", httpx.WithAuth(app, httpx.HandleMyFriends(app)))  // GET

	// 使用者
	mux.HandleFunc("/users/", httpx.HandleUsers(app)) // GET /users/{id}、/posts（公開）、/follow（需登入）

	// CORS 包起來
	handler := httpx.CORS(mux)

	// 啟動
	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}
	addr := ":" + port
	log.Println("Server listening on", addr,
		"DATA_DIR=", cfg.DataDir,
		"NO_AUTH=", config.NoAuth(),
		"FIREBASE_PROJECT_ID=", os.Getenv("FIREBASE_PROJECT_ID"),
		"GOOGLE_APPLICATION_CREDENTIALS=", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
	)
	log.Fatal(http.ListenAndServe(addr, handler))
}
