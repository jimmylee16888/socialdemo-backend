package httpx

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func HandleUpload(app *AppCtx) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 20<<20) // 20MB
		if err := r.ParseMultipartForm(25 << 20); err != nil {
			http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "form file: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		head := make([]byte, 512)
		n, _ := io.ReadFull(file, head)
		head = head[:n]
		mtype := http.DetectContentType(head)

		ext := ""
		switch mtype {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/webp":
			ext = ".webp"
		case "image/gif":
			ext = ".gif"
		default:
			if e := strings.ToLower(filepath.Ext(hdr.Filename)); map[string]bool{
				".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
			}[e] {
				ext = e
			}
			if ext == "" {
				http.Error(w, "unsupported image type: "+mtype, http.StatusBadRequest)
				return
			}
		}

		ts := time.Now().Format("20060102T150405.000")
		base := strings.TrimSuffix(hdr.Filename, filepath.Ext(hdr.Filename))
		if base == "" {
			base = "img"
		}
		base = strings.Map(func(r rune) rune {
			if r == '-' || r == '_' || r == '.' || r == ' ' ||
				(r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				return r
			}
			return '-'
		}, base)
		filename := ts + "_" + base + ext
		dst := filepath.Join(app.Paths.UploadsDir, filename)

		out, err := os.Create(dst)
		if err != nil {
			http.Error(w, "create file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer out.Close()
		if _, err := out.Write(head); err != nil {
			http.Error(w, "write head: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := io.Copy(out, file); err != nil {
			http.Error(w, "write file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if ctype := mime.TypeByExtension(ext); ctype != "" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": "/uploads/" + filename})
	}
}
