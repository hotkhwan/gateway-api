package memsvc

import (
	"context"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/models/grpmod"
	"github.com/hotkhwan/gateway-api/models/memmod"
	"github.com/hotkhwan/gateway-api/models/usrmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"os"

	"go.mongodb.org/mongo-driver/bson"
)

func MembersGetByGroupID(ctx context.Context, groupTree *grpmod.GroupTree) ([]memmod.GroupDetails, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/memsvc",
		"memsvc.GetByGroupID",
		"memsvc", "GetByGroupID",
	)
	defer end()

	groupIDs := CollectUniqueGroupIDs(groupTree)

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	collection := db.Collection("group_members")
	filter := bson.M{"groupID": bson.M{"$in": groupIDs}}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	uniqueMap := make(map[string]memmod.GroupDetails)

	for cursor.Next(ctx) {
		var item memmod.GroupDetails

		if err := cursor.Decode(&item); err != nil {
			return nil, err
		}

		existing, exists := uniqueMap[item.GroupID]

		if !exists || (len(existing.UserIDs) == 0 && len(item.UserIDs) > 0) {
			uniqueMap[item.GroupID] = item
		}
	}

	// แปลง Map กลับเป็น Slice
	var details []memmod.GroupDetails
	for _, v := range uniqueMap {
		details = append(details, v)
	}
	log.Info().Msg("✅ MembersGetByGroupID completed")
	return details, nil
}

func FindInGroupTree(ctx context.Context, nodes []grpmod.GroupTree, groupID string) *grpmod.GroupTree {
	for _, node := range nodes {

		if node.ID == groupID {
			return &node
		}

		if len(node.Children) > 0 {
			found := FindInGroupTree(ctx, node.Children, groupID)
			if found != nil {
				return found
			}
		}
	}
	return nil
}

func CollectUniqueGroupIDs(tree *grpmod.GroupTree) []string {
	idMap := make(map[string]bool)
	var traverse func(node *grpmod.GroupTree)
	traverse = func(node *grpmod.GroupTree) {
		if node == nil {
			return
		}
		if node.ID != "" {
			idMap[node.ID] = true
		}
		for i := range node.Children {
			traverse(&node.Children[i])
		}
	}
	traverse(tree)

	ids := make([]string, 0, len(idMap))
	for id := range idMap {
		ids = append(ids, id)
	}

	return ids
}

func GetUserList(ctx context.Context, page, perPage int, search string) ([]usrmod.KeycloakUser, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/memsvc",
		"memsvc.GetUserList",
		"memsvc", "GetUserList",
	)
	defer end()

	result, err := usrsvc.ListUsers(ctx, "", page, perPage, search, "username", "asc")

	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to get internal user list")
		return nil, err
	}

	return result.Details, nil
}
