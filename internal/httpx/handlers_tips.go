// internal/httpx/handlers_tips.go
package httpx

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TipItem 是前端 TipPrompter 期望的資料格式
type TipItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	ImageURL string `json:"imageUrl,omitempty"` // 可為相對路徑（/uploads/...）或絕對 URL
}

// HandleTipsToday
// GET /tips/today → 回傳「單筆」今日提示；若今天不推播可回 204。
//
// 範例：curl -i 'http://localhost:8088/tips/today?locale=zh-TW'
func HandleTipsToday(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enableJSON(w)
		q := r.URL.Query()

		// 基本存取資訊（方便你在 console 觀察）
		logTipRequest(r, "/tips/today", q)

		// ---- Demo：依日期產生一則固定 tip（實務可改從 DB / 檔案 / 設定抓）
		todayKey := time.Now().Format("2006-01-02") // yyyy-mm-dd
		locale := pickLocale(q.Get("locale"), r.Header.Get("Accept-Language"))

		item := TipItem{
			ID:       "tip_" + todayKey,
			Title:    pickTitleByLocale(locale, "Tip of the Day", "每日小技巧"),
			Body:     pickBodyByLocale(locale, "Long-press a card to share it quickly!", "長按卡片可以快速分享給朋友唷！"),
			ImageURL: "/uploads/tips/share.png", // 放相對路徑，前端會用 baseUrl 補成完整網址
		}

		// 若想偶爾不推播，可改成條件為真時回 204
		// if someCondition {
		// 	w.WriteHeader(http.StatusNoContent)
		// 	return
		// }

		writeJSON(w, http.StatusOK, item)
	}
}

// HandleTipsDaily
// GET /tips/daily → 回傳「多筆」可輪播的提示；前端會自己挑選當天顯示哪一則。
//
// 支援的常見 Query：
//   - clientId, meId, meName, idToken, locale, appVersion, platform
//
// 範例：curl -i 'http://localhost:8088/tips/daily?clientId=dev_xxx&meId=u_abc&meName=Dev&locale=zh-TW'
// HandleTipsDaily
func HandleTipsDaily(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enableJSON(w)
		q := r.URL.Query()

		logTipRequest(r, "/tips/daily", q)

		// ★ 改這段：把宣傳 Banner 放第一筆（或只保留這一筆也行）
		items := []TipItem{
			{
				ID:       "promo_explore_20251028",
				Title:    "WAVE版本  全新推出",
				Body:     "新增雲端訂閱功能，可使用雲端同步所有小卡(製作中)\n支援全卡全圖快照輸出，無須一張張輸入\n以海浪為主題，更加生動簡潔的介面與動畫！",
				ImageURL: "https://jimmylee16888.github.io/popcard-ad/WAVE.png", // ← 改這裡
			},
		}

		writeJSON(w, http.StatusOK, items)
	}
}

//
// ====== 小工具 ======
//

// 開啟 JSON 回應格式
func enableJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, max-age=0") // 讓 tips 每次都能即時更新
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

// 紀錄請求細節，方便除錯
func logTipRequest(r *http.Request, path string, q url.Values) {
	ua := r.Header.Get("User-Agent")
	auth := r.Header.Get("Authorization")
	clientIP := clientIPFromRequest(r)

	// 只截斷顯示前 16 字元，避免 console 太長（你也可以直接印全部）
	authPreview := ""
	if auth != "" {
		if len(auth) > 16 {
			authPreview = auth[:16] + "..."
		} else {
			authPreview = auth
		}
	}

	log.Printf(
		"[%s] ip=%s ua=%s q=%s auth=%s",
		path, clientIP, ua, q.Encode(), authPreview,
	)
}

// 從 X-Forwarded-For / RemoteAddr 抓 IP（在本機/單機也 OK）
func clientIPFromRequest(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// 取第一個
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host := r.RemoteAddr
	// 移除 :port
	if i := strings.LastIndex(host, ":"); i > 0 {
		return host[:i]
	}
	return host
}

// 粗略取用語系（優先 query: locale，再看 Accept-Language）
func pickLocale(queryLocale string, acceptLang string) string {
	if queryLocale != "" {
		return strings.ToLower(queryLocale)
	}
	// Accept-Language: zh-TW,zh;q=0.9,en-US;q=0.8,en;q=0.7
	al := strings.ToLower(acceptLang)
	if strings.HasPrefix(al, "zh") {
		// zh, zh-tw, zh-hant ...
		return "zh-tw"
	}
	return "en"
}

func pickTitleByLocale(locale, en, zh string) string {
	if strings.HasPrefix(locale, "zh") {
		return zh
	}
	return en
}

func pickBodyByLocale(locale, en, zh string) string {
	if strings.HasPrefix(locale, "zh") {
		return zh
	}
	return en
}

// ===== 小提醒 =====
//
// 1) 圖片放置：
//    - 你在 main.go 已掛載：
//         mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(cfg.UploadsDir))))
//      確認 cfg.UploadsDir 內有 tips/xxx.png 檔案即可。
//    - 前端若收到相對路徑（/uploads/...），會用 kSocialBaseUrl 自行補成絕對 URL。
//
// 2) 權限：
//    - 目前兩個端點都未強制驗證（NoAuth/WithAuth 可自行包）。要驗證時可改：
//         mux.HandleFunc("/tips/daily", WithAuth(app, HandleTipsDaily(app)))
//      並在 logTipRequest 中就能看到 Authorization header 的前綴片段。
//
// 3) 回傳格式：
//    - /tips/today：單筆或 204
//    - /tips/daily：多筆陣列。若要根據使用者/好友/標籤客製化，這裡可以讀取 query 或 token 解析後回傳不同內容。
//
// 4) 效能：
//    - Demo 版是動態建構；實務上可做記憶體快取或讀檔案/DB。
//    - 若每日固定一筆，/tips/today 可以在啟動時載入並 cache，到跨日再刷新。
//
// 5) 你想要更細的日誌（例如完整 headers、query map），可用：
//    fmt.Printf("%#v\n", r.Header) 或 log.Printf("%#v\n", q)
