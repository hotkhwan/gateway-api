// internal/services/systemsvc/edgesvc/edgeSsoUrl.go
package edgesvc

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"klynx/internal/repo/stomongo"
	"klynx/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
  errUnsupportedEdgeType = errors.New("unsupported edge type")
  errEdgeNotFound        = errors.New("edge not found")
  errEdgeDeleted         = errors.New("edge is deleted")
  errMissingPassEnc      = errors.New("passEnc missing")
  errMissingSSOBaseURL   = errors.New("missing sso base url")
)

type edgeDoc struct {
  ID        primitive.ObjectID  `bson:"_id"`
  Type      string              `bson:"type"`
  Username  string              `bson:"username"`
  URL       string              `bson:"url"`
  TLS       bool                `bson:"tls"`
  PassEnc   map[string]any      `bson:"passEnc"`
  IsDeleted bool                `bson:"isDeleted"`
  State     string              `bson:"state"`
  DeletedAt *primitive.DateTime `bson:"deletedAt,omitempty"`
}

func BuildEdgeSSOURL(ctx context.Context, edgeType, id, address string) (string, error) {
  oid, err := primitive.ObjectIDFromHex(id)
  if err != nil {
    return "", fmt.Errorf("invalid id: %w", err)
  }

  coll, baseEnv := resolveEdgeCollection(edgeType)
  if coll == "" {
    return "", errUnsupportedEdgeType
  }

  // แนะนำ: ผูก type เข้า filter กันยิงผิดชนิด
  filter := bson.M{"_id": oid, "type": edgeType}

  var doc edgeDoc
  err = stomongo.FindOneProjection(
    ctx,
    coll,
    filter,
    bson.M{
      "_id":       1,
      "type":      1,
      "username":  1,
      "url":       1,
      "tls":       1,
      "passEnc":   1,
      "isDeleted": 1,
      "state":     1,
      "deletedAt": 1,
    },
    &doc,
  )
  if err != nil {
    return "", errEdgeNotFound
  }

  // ป้องกัน record ที่ถูกลบ/archived (ตาม template คุณ)
  if doc.IsDeleted || strings.EqualFold(doc.State, "archived") {
    return "", errEdgeDeleted
  }

  if doc.PassEnc == nil {
    return "", errMissingPassEnc
  }

  passPlain, err := utils.DecryptFromKeyringJSON(doc.PassEnc)
  if err != nil {
    return "", err
  }

  // ✅ requirement: decrypt ก่อน แล้ว base64 password
  passB64 := base64.StdEncoding.EncodeToString([]byte(passPlain))
  // query-escape ให้เป็น %3D
  passParam := url.QueryEscape(passB64)

  base, err := resolveSsoBaseURL(doc, baseEnv)
  if err != nil {
    return "", err
  }

  // url+/#/login?username=...&password=...&address=play
  return buildHashLoginURL(base, doc.Username, passParam, address), nil
}

func resolveEdgeCollection(edgeType string) (collection string, baseEnv string) {
  switch strings.ToLower(strings.TrimSpace(edgeType)) {
  case "svms":
    return "system_edges", "SVMS_SSO_BASE_URL"
  case "ata":
    return "system_edges", "ATA_SSO_BASE_URL"
  default:
    return "", ""
  }
}

func resolveSsoBaseURL(doc edgeDoc, baseEnv string) (string, error) {
  if v := strings.TrimSpace(os.Getenv(baseEnv)); v != "" {
    return normalizeBaseURL(v)
  }
  if strings.TrimSpace(doc.URL) != "" {
    return normalizeBaseURL(doc.URL)
  }
  return "", errMissingSSOBaseURL
}

func normalizeBaseURL(raw string) (string, error) {
  raw = strings.TrimSpace(raw)
  if !strings.Contains(raw, "://") {
    raw = "http://" + raw
  }

  u, err := url.Parse(raw)
  if err != nil {
    return "", err
  }
  if u.Host == "" {
    return "", fmt.Errorf("invalid base url: %s", raw)
  }

  base := url.URL{Scheme: u.Scheme, Host: u.Host}
  return base.String(), nil
}

func buildHashLoginURL(base, username, passwordParam, address string) string {
  b := strings.TrimRight(base, "/")
  return fmt.Sprintf("%s/#/login?username=%s&password=%s&address=%s",
    b,
    url.QueryEscape(username),
    passwordParam,
    url.QueryEscape(address),
  )
}

// ---- error helpers ให้ controller map ได้ชัวร์ ----

func IsUnsupportedEdgeType(err error) bool { return errors.Is(err, errUnsupportedEdgeType) }
func IsNotFound(err error) bool            { return errors.Is(err, errEdgeNotFound) }
func IsDeleted(err error) bool             { return errors.Is(err, errEdgeDeleted) }
