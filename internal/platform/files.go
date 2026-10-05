package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"spotter/internal/core"
)

type Files struct {
	Client *minio.Client
	Bucket string
}

func OpenFiles(endpoint, user, password string) (*Files, error) {
	c, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(user, password, ""), Secure: false})
	if err != nil {
		return nil, err
	}
	return &Files{Client: c, Bucket: "spotter"}, nil
}
func (f *Files) Init(ctx context.Context) error {
	exists, err := f.Client.BucketExists(ctx, f.Bucket)
	if err != nil {
		return err
	}
	if !exists {
		return f.Client.MakeBucket(ctx, f.Bucket, minio.MakeBucketOptions{})
	}
	return nil
}
func (a *API) upload(w http.ResponseWriter, r *http.Request) {
	if a.Files == nil {
		problem(w, core.Fail("SERVICE_UNAVAILABLE", 503, "Хранилище файлов недоступно"))
		return
	}
	name := r.Header.Get("X-File-Name")
	expected := r.Header.Get("X-Content-SHA256")
	if len(name) == 0 || len(name) > 200 || len(expected) != 64 || r.ContentLength < 0 || r.ContentLength > 10<<20 {
		problem(w, core.Fail("VALIDATION_FAILED", 422, "Нужны имя, SHA-256 и длина не больше 10 MiB"))
		return
	}
	id := core.ID()
	_, err := a.Store.DB.Exec(r.Context(), "INSERT INTO files(id,name,size,checksum,status) VALUES($1,$2,$3,$4,'pending')", id, name, r.ContentLength, expected)
	if err != nil {
		problem(w, err)
		return
	}
	h := sha256.New()
	_, err = a.Files.Client.PutObject(r.Context(), a.Files.Bucket, id, io.TeeReader(http.MaxBytesReader(w, r.Body, 10<<20), h), r.ContentLength, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		problem(w, err)
		return
	}
	if hex.EncodeToString(h.Sum(nil)) != expected {
		_ = a.Files.Client.RemoveObject(r.Context(), a.Files.Bucket, id, minio.RemoveObjectOptions{})
		problem(w, core.Fail("CHECKSUM_MISMATCH", 422, "SHA-256 не совпадает"))
		return
	}
	_, err = a.Store.DB.Exec(r.Context(), "UPDATE files SET status='ready' WHERE id=$1", id)
	if err != nil {
		problem(w, err)
		return
	}
	send(w, 200, core.Object{"fileId": id, "status": "ready", "sha256": expected})
}
func (a *API) download(w http.ResponseWriter, r *http.Request) {
	if a.Files == nil {
		problem(w, core.Fail("SERVICE_UNAVAILABLE", 503, "Хранилище файлов недоступно"))
		return
	}
	id := r.PathValue("id")
	var name, checksum string
	var size int64
	err := a.Store.DB.QueryRow(r.Context(), "SELECT name,size,checksum FROM files WHERE id=$1 AND status='ready'", id).Scan(&name, &size, &checksum)
	if err != nil {
		problem(w, core.Fail("ENTITY_NOT_FOUND", 404, "Файл не найден"))
		return
	}
	obj, err := a.Files.Client.GetObject(r.Context(), a.Files.Bucket, id, minio.GetObjectOptions{})
	if err != nil {
		problem(w, err)
		return
	}
	defer obj.Close()
	if _, err = obj.Stat(); err != nil {
		problem(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("X-Content-SHA256", checksum)
	_, _ = io.Copy(w, obj)
}
