package optsvc

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"klynx/config"
	"klynx/models/optmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func extractNS(kindDotID string, kind string) string {
	return strings.TrimPrefix(kindDotID, kind+".")
}

// ---- helpers: prune & sort ----

// ตัดฟิลด์หนักๆ เมื่อ "คืนทั้งหมด"
func pruneHeavyFields(m bson.M) {
	delete(m, "policeProvincial")
	delete(m, "policeStation")
}

// เรียง slice ของ []map[string]any หรือ bson.A ตาม title (ถ้าเจอ)
func sortOptionArrayAscByTitle(val any) any {
	switch arr := val.(type) {
	case []map[string]any:
		sort.Slice(arr, func(i, j int) bool {
			ti, _ := arr[i]["title"].(string)
			tj, _ := arr[j]["title"].(string)
			return strings.Compare(ti, tj) < 0
		})
		return arr
	case []bson.M:
		sort.Slice(arr, func(i, j int) bool {
			ti, _ := arr[i]["title"].(string)
			tj, _ := arr[j]["title"].(string)
			return strings.Compare(ti, tj) < 0
		})
		return arr
	case bson.A:
		// แปลงเป็น []bson.M ก่อน
		tmp := make([]bson.M, 0, len(arr))
		for _, it := range arr {
			if mm, ok := it.(bson.M); ok {
				tmp = append(tmp, mm)
			}
		}
		sort.Slice(tmp, func(i, j int) bool {
			ti, _ := tmp[i]["title"].(string)
			tj, _ := tmp[j]["title"].(string)
			return strings.Compare(ti, tj) < 0
		})
		out := make(bson.A, 0, len(tmp))
		for _, it := range tmp {
			out = append(out, it)
		}
		return out
	default:
		return val
	}
}

// เรียง policeRegion / policeProvincial / policeStation ทั้งก้อนในเอกสาร
func sortAllKnownListsInDoc(m bson.M) {
	// policeRegion: เป็นอาเรย์ชั้นเดียว
	if v, ok := m["policeRegion"]; ok {
		m["policeRegion"] = sortOptionArrayAscByTitle(v)
	}
	// policeProvincial: เป็น map[regionId] -> []{id,title}
	if mp, ok := m["policeProvincial"].(bson.M); ok {
		for k, v := range mp {
			mp[k] = sortOptionArrayAscByTitle(v)
		}
	}
	if mp2, ok := m["policeProvincial"].(map[string]any); ok {
		for k, v := range mp2 {
			mp2[k] = sortOptionArrayAscByTitle(v)
		}
	}
	// policeStation: เป็น map[provId] -> []{id,title}
	if mp, ok := m["policeStation"].(bson.M); ok {
		for k, v := range mp {
			mp[k] = sortOptionArrayAscByTitle(v)
		}
	}
	if mp2, ok := m["policeStation"].(map[string]any); ok {
		for k, v := range mp2 {
			mp2[k] = sortOptionArrayAscByTitle(v)
		}
	}
}

// -------- fetchers --------

// กรณีไม่มี ns → คืนทุก namespace (ตัดฟิลด์หนัก + เรียงให้)
func GetAllOptions(ctx context.Context, kind string) (optmod.NamespacedOptions, error) {
	col := config.DB.Collection(collOptions)

	reg := fmt.Sprintf("^%s\\.", regexp.QuoteMeta(kind))
	cur, err := col.Find(ctx, bson.M{"_id": bson.M{"$regex": reg}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	out := optmod.NamespacedOptions{}
	for cur.Next(ctx) {
		var m bson.M
		if err := cur.Decode(&m); err != nil {
			return nil, err
		}
		id, _ := m["_id"].(string)
		ns := extractNS(id, kind)

		// ลบเมตา, ตัดฟิลด์หนัก, และเรียง
		delete(m, "_id")
		delete(m, "createdAt")
		delete(m, "updatedAt")
		pruneHeavyFields(m)
		sortAllKnownListsInDoc(m)

		out[ns] = map[string]any(m)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ResolveRegionCanonicalID แปลงค่าที่ผู้ใช้ส่งมา (id หรือ code) -> regionID (string)
// ใช้กับ policeRegion เพื่อให้รับได้ทั้ง id (uuid) และ code (ตัวเลข)
func ResolveRegionCanonicalID(ctx context.Context, kind, ns, val string) (string, error) {
	col := config.DB.Collection(collOptions)

	findOpts := options.FindOne().SetProjection(bson.M{"_id": 0, "policeRegion": 1})

	var m bson.M
	err := col.FindOne(ctx, bson.M{"_id": docID(kind, ns)}, findOpts).Decode(&m)
	if err == mongo.ErrNoDocuments {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	arrAny := m["policeRegion"]
	switch arr := arrAny.(type) {
	case bson.A:
		for _, it := range arr {
			obj, _ := it.(bson.M)
			if obj == nil {
				continue
			}
			idStr := fmt.Sprint(obj["id"])
			codeStr := fmt.Sprint(obj["code"]) // รองรับทั้ง number และ string

			if idStr != "" && idStr == val {
				return idStr, nil
			}
			if codeStr != "" && codeStr == val {
				return idStr, nil
			}
		}
	case []bson.M:
		for _, obj := range arr {
			idStr := fmt.Sprint(obj["id"])
			codeStr := fmt.Sprint(obj["code"])
			if idStr == val || codeStr == val {
				return idStr, nil
			}
		}
	case []map[string]any:
		for _, obj := range arr {
			idStr := fmt.Sprint(obj["id"])
			codeStr := fmt.Sprint(obj["code"])
			if idStr == val || codeStr == val {
				return idStr, nil
			}
		}
	}

	return "", nil
}

// ResolveProvincialCanonicalID แปลงค่าที่ผู้ใช้ส่งมา (id หรือ code) -> provincialID (string)
// ใช้กับ policeProvincial เพื่อให้รับได้ทั้ง id (uuid) และ code (ตัวเลข)
func ResolveProvincialCanonicalID(ctx context.Context, kind, ns, val string) (string, error) {
	col := config.DB.Collection(collOptions)

	findOpts := options.FindOne().SetProjection(bson.M{"_id": 0, "policeProvincial": 1})
	var m bson.M
	err := col.FindOne(ctx, bson.M{"_id": docID(kind, ns)}, findOpts).Decode(&m)
	if err == mongo.ErrNoDocuments {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	pp, _ := m["policeProvincial"].(bson.M)
	for _, arrAny := range pp {
		arr, _ := arrAny.(bson.A)
		for _, it := range arr {
			obj, _ := it.(bson.M)
			if obj == nil {
				continue
			}
			idStr := fmt.Sprint(obj["id"])
			codeStr := fmt.Sprint(obj["code"])

			if idStr == val || codeStr == val {
				return idStr, nil
			}
		}
	}
	return "", nil
}

// ดึง namespace เดียว
// - ถ้า name == "" (ขอทั้ง namespace) → ตัดฟิลด์หนัก + เรียง
// - ถ้า name != "" (ขอฟิลด์เดียว) → project ฟิลด์นั้นและเรียงภายในถ้าเป็นลิสต์
func GetNamespaceOptions(ctx context.Context, kind, ns, name string) (optmod.NamespacedOptions, error) {
	col := config.DB.Collection(collOptions)

	findOpts := options.FindOne()
	if name != "" {
		findOpts = findOpts.SetProjection(bson.M{
			"_id": 0,
			name:  1,
		})
	}

	var m bson.M
	err := col.FindOne(ctx, bson.M{"_id": docID(kind, ns)}, findOpts).Decode(&m)
	if err == mongo.ErrNoDocuments {
		return optmod.NamespacedOptions{ns: {}}, nil
	}
	if err != nil {
		return nil, err
	}

	if name == "" {
		// ทั้ง namespace → ตัดฟิลด์หนัก + เรียง
		delete(m, "_id")
		delete(m, "createdAt")
		delete(m, "updatedAt")
		pruneHeavyFields(m)
		sortAllKnownListsInDoc(m)
		return optmod.NamespacedOptions{ns: map[string]any(m)}, nil
	}

	// name เดียว → พยายามเรียงถ้าเป็นหนึ่งในลิสต์ที่รู้จัก
	if v, ok := m[name]; ok {
		// ถ้าเป็น policeRegion (array) ให้เรียง
		// ถ้าเป็น policeProvincial / policeStation (map[string][]items) ให้เรียงทุกคีย์
		switch name {
		case "policeRegion":
			m[name] = sortOptionArrayAscByTitle(v)
		case "policeProvincial", "policeStation":
			// กรณีขอ name เดียว เรา "ไม่ตัด" เพราะ user อยากได้ฟิลด์นี้อยู่แล้ว
			if mp, ok := v.(bson.M); ok {
				for k, vv := range mp {
					mp[k] = sortOptionArrayAscByTitle(vv)
				}
				m[name] = mp
			} else if mp2, ok := v.(map[string]any); ok {
				for k, vv := range mp2 {
					mp2[k] = sortOptionArrayAscByTitle(vv)
				}
				m[name] = mp2
			}
		}
	}

	return optmod.NamespacedOptions{ns: map[string]any(m)}, nil
}

// ดึง field เดียว + sub-path (เช่น policeProvincial.<regionId>)
// คืนเฉพาะก้อนย่อยนั้น และ "เรียง" ให้ด้วย
func GetNamespaceOptionsPath(ctx context.Context, kind, ns, name, path string) (optmod.NamespacedOptions, error) {
	col := config.DB.Collection(collOptions)

	sub := path
	if strings.HasPrefix(sub, name+".") {
		sub = strings.TrimPrefix(sub, name+".")
	}
	projectKey := fmt.Sprintf("%s.%s", name, sub)

	findOpts := options.FindOne().SetProjection(bson.M{
		"_id":      0,
		projectKey: 1,
	})

	var m bson.M
	err := col.FindOne(ctx, bson.M{"_id": docID(kind, ns)}, findOpts).Decode(&m)
	if err == mongo.ErrNoDocuments {
		return optmod.NamespacedOptions{ns: {}}, nil
	}
	if err != nil {
		return nil, err
	}

	// normalize { ns: { name: valueSorted } }
	var val any
	if top, ok := m[name].(bson.M); ok {
		val = top[sub]
	} else if top2, ok := m[name].(map[string]any); ok {
		val = top2[sub]
	}

	// เรียงรายการย่อยตาม title
	val = sortOptionArrayAscByTitle(val)

	return optmod.NamespacedOptions{
		ns: map[string]any{
			name: val,
		},
	}, nil
}

// ---- resolvers (เดิม) ----

func ResolveProvincialByStation(ctx context.Context, kind, ns, stationID string) (provincialID string, stationObj map[string]any, err error) {
	col := config.DB.Collection(collOptions)

	findOpts := options.FindOne().SetProjection(bson.M{"_id": 0, "policeStation": 1})
	var m bson.M
	err = col.FindOne(ctx, bson.M{"_id": docID(kind, ns)}, findOpts).Decode(&m)
	if err == mongo.ErrNoDocuments {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}

	ps, _ := m["policeStation"].(bson.M)
	for prov, arrAny := range ps {
		arr, _ := arrAny.(bson.A)
		for _, it := range arr {
			obj, _ := it.(bson.M)
			if fmt.Sprint(obj["id"]) == stationID {
				return prov, map[string]any(obj), nil
			}
		}
	}
	return "", nil, nil
}

func ResolveRegionByProvincial(ctx context.Context, kind, ns, provincialID string) (string, error) {
	col := config.DB.Collection(collOptions)

	findOpts := options.FindOne().SetProjection(bson.M{"_id": 0, "policeProvincial": 1})
	var m bson.M
	err := col.FindOne(ctx, bson.M{"_id": docID(kind, ns)}, findOpts).Decode(&m)
	if err == mongo.ErrNoDocuments {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	pp, _ := m["policeProvincial"].(bson.M)
	for reg, arrAny := range pp {
		arr, _ := arrAny.(bson.A)
		for _, it := range arr {
			obj, _ := it.(bson.M)
			if fmt.Sprint(obj["id"]) == provincialID {
				return reg, nil
			}
		}
	}
	return "", nil
}
