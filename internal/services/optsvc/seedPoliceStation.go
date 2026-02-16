package optsvc

import (
	"context"
	"sort"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/optmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SeedPoliceStation
//   - รับ raw items (level 1..3)
//   - จัดกลุ่มโดยใช้ "id" เป็น key (ไม่อิง code)
//   - ผลลัพธ์ structure:
//     policeRegion:       []{id,title}
//     policeProvincial:   map[regionId][]{{id,title}...}
//     policeStation:      map[provincialId][]{{id,title}...}
//   - ถ้า write=true จะ upsert เข้า Mongo (_id="list.<ns>")
func SeedPoliceStation(ctx context.Context, ns string, raws []optmod.StationRaw, write bool) (map[string]any, error) {
	// buckets
	regions := make([]map[string]any, 0, 64)               // level 1
	provByRegion := make(map[string][]map[string]any, 128) // level 2, key=regionId
	staByProv := make(map[string][]map[string]any, 256)    // level 3, key=provId

	// index (ถ้าต้องการ validate parent ต่อในอนาคต)
	regionIDs := make(map[string]struct{}, 128)
	provIDs := make(map[string]struct{}, 512)

	// รอบแรก: เก็บ region/provincial/station ลง bucket ตาม level
	for _, r := range raws {
		title := r.Name
		switch r.Level {
		case 1: // Region
			regions = append(regions, map[string]any{
				"id":    r.ID,
				"title": title,
				"code":  int(r.Code), // 👈 แปลงเป็น int ชัดเจน
			})
			regionIDs[r.ID] = struct{}{}

		case 2: // Provincial
			pid := r.GetParentID()
			if pid == nil || *pid == "" {
				continue
			}
			provIDs[r.ID] = struct{}{}
			provByRegion[*pid] = append(provByRegion[*pid], map[string]any{
				"id":    r.ID,
				"title": title,
				"code":  int(r.Code), // 👈
			})

		case 3: // Station
			pid := r.GetParentID()
			if pid == nil || *pid == "" {
				continue
			}
			staByProv[*pid] = append(staByProv[*pid], map[string]any{
				"id":    r.ID,
				"title": title,
				"code":  int(r.Code), // 👈
			})
		default:
			// ข้าม level อื่น
		}
	}

	// sort ตาม title เหมือนเดิม
	sort.Slice(regions, func(i, j int) bool {
		return regions[i]["title"].(string) < regions[j]["title"].(string)
	})
	for k := range provByRegion {
		arr := provByRegion[k]
		sort.Slice(arr, func(i, j int) bool { return arr[i]["title"].(string) < arr[j]["title"].(string) })
		provByRegion[k] = arr
	}
	for k := range staByProv {
		arr := staByProv[k]
		sort.Slice(arr, func(i, j int) bool { return arr[i]["title"].(string) < arr[j]["title"].(string) })
		staByProv[k] = arr
	}

	// payload ที่จะคืน/เขียน
	payload := map[string]any{
		"policeRegion":     regions,      // []{id,title,code}
		"policeProvincial": provByRegion, // map[regionId][]{{id,title,code}...}
		"policeStation":    staByProv,    // map[provincialId][]{{id,title,code}...}
	}

	// เขียนลง Mongo (upsert)
	if write {
		col := config.DB.Collection(collOptions)
		up := true
		_, err := col.UpdateByID(ctx, docID("list", ns),
			bson.M{
				"$set": payload,
				"$setOnInsert": bson.M{
					"_id": docID("list", ns),
				},
			},
			&options.UpdateOptions{Upsert: &up},
		)
		if err != nil {
			return nil, err
		}
	}

	return payload, nil
}
