package control

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/update"
)

func storageCipher() (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(os.Getenv("NODELANE_UPDATE_STORAGE_KEY"))
	if err != nil || len(key) != 32 {
		return nil, errors.New("NODELANE_UPDATE_STORAGE_KEY must contain a shared base64-encoded 32-byte encryption key")
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func encryptSource(id, access, secret string) ([]byte, error) {
	c, err := storageCipher()
	if err != nil {
		return nil, err
	}
	plain, _ := json.Marshal([]string{access, secret})
	nonce := make([]byte, c.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.Seal(nonce, nonce, plain, []byte(id)), nil
}

func readSource(ctx context.Context, tx pgx.Tx, id string, secrets bool) (model.UpdateSource, error) {
	var src model.UpdateSource
	var raw, sealed []byte
	err := tx.QueryRow(ctx, "SELECT config,secret,revision FROM update_sources WHERE id=$1", id).Scan(&raw, &sealed, &src.Revision)
	if err != nil {
		return src, noRows(err)
	}
	revision := src.Revision
	if err = json.Unmarshal(raw, &src); err != nil {
		return src, err
	}
	src.Revision, src.ID = revision, id
	src.HasCredentials = len(sealed) > 0
	if secrets && len(sealed) > 0 {
		c, err := storageCipher()
		if err != nil {
			return src, err
		}
		if len(sealed) < c.NonceSize() {
			return src, errors.New("invalid encrypted update credentials")
		}
		b, err := c.Open(nil, sealed[:c.NonceSize()], sealed[c.NonceSize():], []byte(id))
		if err != nil {
			return src, errors.New("cannot decrypt update credentials")
		}
		var pair []string
		if json.Unmarshal(b, &pair) != nil || len(pair) != 2 {
			return src, errors.New("invalid update credentials")
		}
		src.AccessKey, src.SecretKey = pair[0], pair[1]
	}
	return src, nil
}

func saveSource(ctx context.Context, tx pgx.Tx, actor string, src model.UpdateSource) (any, error) {
	if src.ID == "" {
		src.ID = randomID()
	}
	if !validResourceID(src.ID) || len(src.Name) == 0 || len(src.Name) > 80 || (src.Kind != "r2" && src.Kind != "s3" && src.Kind != "https") || len(src.Prefix) > 200 || strings.Contains(src.Prefix, "..") || strings.ContainsAny(src.Prefix, "\\?#") || src.Priority < 0 || src.Priority > 1000 {
		return nil, ErrInvalid
	}
	if src.PublicURL != "" && !update.ValidURL(src.PublicURL) {
		return nil, ErrInvalid
	}
	if src.Kind == "https" {
		if src.PublicURL == "" || src.AccessKey != "" || src.SecretKey != "" {
			return nil, ErrInvalid
		}
	} else {
		if !update.ValidURL(src.Endpoint) || src.Bucket == "" || strings.ContainsAny(src.Bucket, "/\\?#") || len(src.Bucket) > 255 {
			return nil, ErrInvalid
		}
		if src.Region == "" {
			src.Region = "auto"
		}
	}
	var sealed []byte
	var oldConfig []byte
	var oldRevision int64
	err := tx.QueryRow(ctx, "SELECT secret,revision,config FROM update_sources WHERE id=$1", src.ID).Scan(&sealed, &oldRevision, &oldConfig)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if src.Revision != oldRevision {
		return nil, ErrConflict
	}
	if oldRevision == 0 {
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM update_sources").Scan(&count); err != nil {
			return nil, err
		}
		if count >= 16 {
			return nil, ErrConflict
		}
	}
	if (src.AccessKey == "") != (src.SecretKey == "") {
		return nil, ErrInvalid
	}
	changedCredentials := src.AccessKey != ""
	if changedCredentials {
		sealed, err = encryptSource(src.ID, src.AccessKey, src.SecretKey)
		if err != nil {
			return nil, err
		}
	}
	if src.Kind == "https" {
		sealed = []byte{}
	} else if len(sealed) == 0 {
		return nil, ErrInvalid
	}
	src.AccessKey, src.SecretKey = "", ""
	src.HasCredentials = len(sealed) > 0
	src.Revision++
	b, _ := json.Marshal(src)
	_, err = tx.Exec(ctx, `INSERT INTO update_sources(id,config,secret,revision) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET config=$2,secret=$3,revision=$4`, src.ID, b, sealed, src.Revision)
	var old model.UpdateSource
	if err == nil && oldRevision > 0 && json.Unmarshal(oldConfig, &old) == nil && !changedCredentials && old.Kind == src.Kind && old.Endpoint == src.Endpoint && old.Bucket == src.Bucket && old.Region == src.Region && old.Prefix == src.Prefix && old.PublicURL == src.PublicURL && old.PathStyle == src.PathStyle {
		_, err = tx.Exec(ctx, "UPDATE update_replicas SET source_revision=$3 WHERE source_id=$1 AND source_revision=$2", src.ID, oldRevision, src.Revision)
	}
	if err == nil && oldRevision > 0 {
		rows, e := tx.Query(ctx, "SELECT DISTINCT release_id FROM update_policies WHERE release_id IS NOT NULL")
		if e != nil {
			return nil, e
		}
		ids, e := pgx.CollectRows(rows, pgx.RowTo[string])
		if e != nil {
			return nil, e
		}
		for _, id := range ids {
			r, e := readRelease(ctx, tx, id)
			if e != nil {
				return nil, e
			}
			if _, e = releaseURLs(ctx, tx, r); e != nil {
				return nil, fmt.Errorf("%w: verify a backup source or remove its policy before changing the last source", ErrConflict)
			}
		}
	}
	if err == nil {
		err = adminEvent(ctx, tx, actor, "update.source_saved", src.ID, map[string]any{"revision": src.Revision})
	}
	return src, err
}

func validResourceID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func sourceClient(src model.UpdateSource) *s3.Client {
	h := update.HTTPClient()
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return s3.New(s3.Options{Region: src.Region, BaseEndpoint: aws.String(src.Endpoint), UsePathStyle: src.PathStyle, Credentials: credentials.NewStaticCredentialsProvider(src.AccessKey, src.SecretKey, ""), HTTPClient: h, RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired})
}

func sourceURL(ctx context.Context, src model.UpdateSource, target string) (string, error) {
	key := update.ObjectKey(src.Prefix, target)
	if src.PublicURL != "" {
		return strings.TrimRight(src.PublicURL, "/") + "/" + strings.Join(escapeParts(key), "/"), nil
	}
	u, err := s3.NewPresignClient(sourceClient(src)).PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(src.Bucket), Key: aws.String(key)}, func(o *s3.PresignOptions) { o.Expires = 2 * time.Hour })
	if err != nil {
		return "", errors.New("update source signing failed")
	}
	return u.URL, nil
}
func escapeParts(key string) []string {
	p := strings.Split(key, "/")
	for i := range p {
		p[i] = url.PathEscape(p[i])
	}
	return p
}

func verifyReplica(ctx context.Context, src model.UpdateSource, a model.UpdateArtifact) error {
	address, err := sourceURL(ctx, src, a.Target)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return ErrInvalid
	}
	res, err := update.HTTPClient().Do(req)
	if err != nil {
		return errors.New("update source unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("update package unavailable")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(res.Body, a.Size+1))
	if err != nil || n != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return errors.New("update package verification failed")
	}
	return nil
}
