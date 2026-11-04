package nexhost

// S3 half of account erasure: the bodies of DataStore objects live in a bucket under
// "<key base>/<data id>.bin" (nex-protocols-common-go), the key base differing by title. The bucket
// is therefore listed once and every "<digits>.bin" whose id belongs to a removed object is deleted,
// whatever its prefix.

import (
	"context"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Target is one bucket a title stores object bodies in.
type S3Target struct {
	Endpoint, Bucket, Access, Secret string
	Secure                           bool
}

var bodyKey = regexp.MustCompile(`(?:^|/)(\d+)\.bin$`)

func (t S3Target) client() (*minio.Client, error) {
	ep := t.Endpoint
	secure := t.Secure
	if u, err := url.Parse(ep); err == nil && u.Host != "" {
		ep, secure = u.Host, u.Scheme == "https"
	}
	return minio.New(ep, &minio.Options{Creds: credentials.NewStaticV4(t.Access, t.Secret, ""), Secure: secure})
}

// deleteObjects removes the bodies of the data ids from the bucket. A missing bucket is nothing to do.
func (t S3Target) deleteObjects(ctx context.Context, ids []int64) error {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[strconv.FormatInt(id, 10)] = true
	}
	c, err := t.client()
	if err != nil {
		return err
	}
	for obj := range c.ListObjects(ctx, t.Bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			if minio.ToErrorResponse(obj.Err).Code == "NoSuchBucket" {
				return nil
			}
			return obj.Err
		}
		m := bodyKey.FindStringSubmatch(obj.Key)
		if m == nil || !want[m[1]] {
			continue
		}
		if err := c.RemoveObject(ctx, t.Bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil &&
			minio.ToErrorResponse(err).Code != "NoSuchKey" {
			return err
		}
	}
	return ctx.Err()
}

var s3Var = regexp.MustCompile(`^(.*?)S3_?(ENDPOINT|URL|BUCKET|ACCESS_KEY|KEY|SECRET_KEY|ACCESS_SECRET|SECRET|INSECURE|SECURE|USE_TLS)$`)

// s3TargetsFromEnv finds the buckets in the environment. Titles name their variables differently
// (ACNH_S3_BUCKET, PN_SMM_CONFIG_S3_BUCKET, PN_SSBWIIU_DATASTORE_S3BUCKET, DATASTORE_S3_BUCKET), so
// the variables are grouped by the text before "S3"; a group that has a bucket but no endpoint or
// credentials takes them from the unprefixed S3_* group (the shared MinIO account).
func s3TargetsFromEnv() []S3Target {
	groups := map[string]*S3Target{}
	get := func(p string) *S3Target {
		if groups[p] == nil {
			groups[p] = &S3Target{}
		}
		return groups[p]
	}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		m := s3Var.FindStringSubmatch(k)
		if m == nil || v == "" {
			continue
		}
		v = os.ExpandEnv(v)
		g := get(m[1])
		switch m[2] {
		case "ENDPOINT", "URL":
			g.Endpoint = v
		case "BUCKET":
			g.Bucket = v
		case "ACCESS_KEY", "KEY":
			g.Access = v
		case "SECRET_KEY", "ACCESS_SECRET", "SECRET":
			g.Secret = v
		case "INSECURE":
			g.Secure = !(v == "1" || strings.EqualFold(v, "true"))
		case "SECURE", "USE_TLS":
			g.Secure = v == "1" || strings.EqualFold(v, "true")
		}
	}
	shared := groups[""]
	var out []S3Target
	seen := map[S3Target]bool{}
	for _, g := range groups {
		if g.Bucket == "" {
			continue
		}
		t := *g
		if shared != nil {
			if t.Endpoint == "" {
				t.Endpoint, t.Secure = shared.Endpoint, shared.Secure
			}
			if t.Access == "" {
				t.Access = shared.Access
			}
			if t.Secret == "" {
				t.Secret = shared.Secret
			}
		}
		if t.Endpoint == "" || t.Access == "" || t.Secret == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}
