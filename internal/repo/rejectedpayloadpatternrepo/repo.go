// internal/repo/rejectedpayloadpatternrepo/repo.go
package rejectedpayloadpatternrepo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const col = "rejected_payload_patterns"

var ErrNotFound = errors.New("rejected payload pattern not found")
var ErrAlreadyExists = errors.New("rejected payload pattern already exists")

type RejectedPayloadPatternRepo struct{}

func NewRejectedPayloadPatternRepo() *RejectedPayloadPatternRepo {
	return &RejectedPayloadPatternRepo{}
}

// Create inserts a new rejected pattern. Returns ErrAlreadyExists if the pair already exists.
func (r *RejectedPayloadPatternRepo) Create(
	ctx context.Context,
	orgId, sourceFamily, fingerprint, reason, createdBy string,
) (string, error) {
	// check duplicate
	exists, err := r.Exists(ctx, orgId, sourceFamily, fingerprint)
	if err != nil {
		return "", err
	}
	if exists {
		return "", ErrAlreadyExists
	}

	patternId := uuid.NewString()
	doc := &ingestmod.RejectedPayloadPattern{
		PatternId:    patternId,
		OrgId:        orgId,
		SourceFamily: sourceFamily,
		Fingerprint:  fingerprint,
		Reason:       reason,
		CreatedBy:    createdBy,
		CreatedAt:    time.Now().UTC(),
	}
	if _, err := stomongo.InsertOne(ctx, col, doc); err != nil {
		return "", err
	}
	return patternId, nil
}

// Exists checks if a pattern with given orgId+sourceFamily+fingerprint already exists.
func (r *RejectedPayloadPatternRepo) Exists(ctx context.Context, orgId, sourceFamily, fingerprint string) (bool, error) {
	filter := bson.M{
		"orgId":        orgId,
		"sourceFamily": sourceFamily,
		"fingerprint":  fingerprint,
	}
	var doc ingestmod.RejectedPayloadPattern
	err := stomongo.FindOne(ctx, col, filter, &doc)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return false, err
}

// FindById finds a pattern by orgId+patternId.
func (r *RejectedPayloadPatternRepo) FindById(ctx context.Context, orgId, patternId string) (*ingestmod.RejectedPayloadPattern, error) {
	filter := bson.M{"orgId": orgId, "patternId": patternId}
	var doc ingestmod.RejectedPayloadPattern
	if err := stomongo.FindOne(ctx, col, filter, &doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &doc, nil
}

// List returns paginated patterns for an org, sorted by createdAt desc.
func (r *RejectedPayloadPatternRepo) List(ctx context.Context, orgId string, page, perPage int) ([]*ingestmod.RejectedPayloadPattern, *gmod.Pagination, error) {
	filter := bson.M{"orgId": orgId}
	sort := bson.D{{Key: "createdAt", Value: -1}}
	var items []*ingestmod.RejectedPayloadPattern
	pag, err := stomongo.FindWithPagination(ctx, col, filter, sort, page, perPage, &items)
	if err != nil {
		return nil, nil, err
	}
	return items, &pag, nil
}

// Delete hard-deletes a pattern by orgId+patternId.
func (r *RejectedPayloadPatternRepo) Delete(ctx context.Context, orgId, patternId string) error {
	filter := bson.M{"orgId": orgId, "patternId": patternId}
	result, err := stomongo.DeleteOne(ctx, col, filter)
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}
