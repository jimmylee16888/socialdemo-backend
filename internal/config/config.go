package config

import (
	"context"
	"log"
	"os"
	"path/filepath"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

type Paths struct {
	DataDir      string
	UploadsDir   string
	PostsFile    string
	TagsFile     string
	FriendsFile  string
	ProfilesFile string
	LikesFile    string
}

func DefaultPaths() Paths {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
		if _, err := os.Stat(dataDir); err != nil {
			dataDir = filepath.Join(".", "data")
		}
	}
	return Paths{
		DataDir:      dataDir,
		UploadsDir:   filepath.Join(dataDir, "uploads"),
		PostsFile:    filepath.Join(dataDir, "posts.json"),
		TagsFile:     filepath.Join(dataDir, "tags.json"),
		FriendsFile:  filepath.Join(dataDir, "friends.json"),
		ProfilesFile: filepath.Join(dataDir, "profiles.json"),
		LikesFile:    filepath.Join(dataDir, "likes.json"),
	}
}

func EnsureDir(dir string) { _ = os.MkdirAll(dir, 0o755) }

func NoAuth() bool { return os.Getenv("NO_AUTH") == "1" }

// Firebase Auth（保留；NO_AUTH=1 則不啟用）
func NewAuthClient() *auth.Client {
	if NoAuth() {
		return nil
	}
	proj := os.Getenv("FIREBASE_PROJECT_ID")
	if proj == "" {
		log.Fatal("FIREBASE_PROJECT_ID not set")
	}

	var opts []option.ClientOption
	if saJSON := os.Getenv("FIREBASE_SERVICE_ACCOUNT_JSON"); saJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(saJSON)))
	} else if cred := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); cred != "" {
		if _, err := os.Stat(cred); err != nil {
			log.Fatalf("GOOGLE_APPLICATION_CREDENTIALS %q not readable: %v", cred, err)
		}
		opts = append(opts, option.WithCredentialsFile(cred))
	} else if os.Getenv("FIREBASE_AUTH_EMULATOR_HOST") == "" {
		log.Fatal("Missing Firebase credentials. Set FIREBASE_SERVICE_ACCOUNT_JSON or GOOGLE_APPLICATION_CREDENTIALS, or use FIREBASE_AUTH_EMULATOR_HOST / NO_AUTH=1")
	}

	app, err := firebase.NewApp(context.Background(), &firebase.Config{
		ProjectID: proj,
	}, opts...)
	if err != nil {
		log.Fatalf("firebase init: %v", err)
	}
	client, err := app.Auth(context.Background())
	if err != nil {
		log.Fatalf("firebase auth: %v", err)
	}
	return client
}
