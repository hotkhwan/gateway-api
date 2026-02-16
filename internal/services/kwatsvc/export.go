// internal/services/kwatsvc/export.go
package kwatsvc

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/internal/repo/stos3minio"
	"github.com/hotkhwan/gateway-api/models/kwatmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

const (
	exportJobsColl = "kwatch_watchlist_exports_jobs" // เก็บสถานะงาน export
)

// ======================= Public API =======================

func StartWatchlistExport(ctx context.Context, p kwatmod.ExportWatchlistParams) (string, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kwatsvc", "StartWatchlistExport", "kwatsvc", "export-start")
	defer end()

	now := time.Now()
	job := kwatmod.ExportJob{
		Kind:      kwatmod.ExportKindWatchlist,
		Status:    kwatmod.ExportStatusPending,
		Params:    p,
		CreatedAt: now,
	}
	oid, err := stomongo.InsertOne(ctx, exportJobsColl, job)
	if err != nil {
		return "", err
	}
	jobID := oid.Hex()
	log.Info().Str("jobID", jobID).Msg("created export job")

	// run async
	go func(parent context.Context, id string, params kwatmod.ExportWatchlistParams) {
		ctx2, end2, lg := traceutil.StartLite(parent, "github.com/hotkhwan/gateway-api/kwatsvc", "RunExport", "kwatsvc", "export-run")
		defer end2()

		objID, _ := primitive.ObjectIDFromHex(id)
		_, _ = stomongo.UpdateOne(ctx2, exportJobsColl, bson.M{"_id": objID}, bson.M{
			"status":    kwatmod.ExportStatusRunning,
			"startedAt": time.Now(),
		})

		res, err := runWatchlist(ctx2, id, params)
		if err != nil {
			_, _ = stomongo.UpdateOne(ctx2, exportJobsColl, bson.M{"_id": objID}, bson.M{
				"status":  kwatmod.ExportStatusFailed,
				"endedAt": time.Now(),
				"error":   err.Error(),
			})
			lg.Error().Err(err).Str("jobID", id).Msg("export failed")
			return
		}
		_, _ = stomongo.UpdateOne(ctx2, exportJobsColl, bson.M{"_id": objID}, bson.M{
			"status":  kwatmod.ExportStatusSucceeded,
			"endedAt": time.Now(),
			"result":  res,
		})
		lg.Info().Str("jobID", id).Str("key", res.Key).Msg("export done")
	}(ctx, jobID, p)

	return jobID, nil
}

func GetJob(ctx context.Context, id string) (*kwatmod.ExportJob, error) {
	var job kwatmod.ExportJob
	objID, _ := primitive.ObjectIDFromHex(id)
	if err := stomongo.FindOne(ctx, exportJobsColl, bson.M{"_id": objID}, &job); err != nil {
		return nil, errors.New("job not found")
	}
	return &job, nil
}

// ======================= Worker =======================

// task สำหรับดึงรูปไปใส่ ZIP
type imgTask struct {
	DocID     string
	FaceKey   string // อาจว่าง
	OriginKey string // อาจว่าง
	FaceExt   string // .jpg/.png (เอาจาก key)
	OriginExt string
}

func runWatchlist(ctx context.Context, jobID string, p kwatmod.ExportWatchlistParams) (*kwatmod.ExportResult, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kwatsvc", "BuildWatchlist", "kwatsvc", "export-build")
	defer end()

	// limit: 0 = ไม่จำกัด (stream ทั้งหมด)
	useLimit := p.Limit > 0

	// ---------- สร้าง filter ----------
	filter := bson.M{}
	filter["isDeleted"] = bson.M{"$ne": true}
	if s := strings.TrimSpace(p.Search); s != "" {
		filter["$or"] = []bson.M{
			{"idcard": bson.M{"$regex": s, "$options": "i"}},
			{"fullName": bson.M{"$regex": s, "$options": "i"}},
			{"personKey": bson.M{"$regex": s, "$options": "i"}},
		}
	}
	if p.From != "" || p.To != "" {
		r := bson.M{}
		if p.From != "" {
			if t, e := time.Parse("2006-01-02", p.From); e == nil {
				r["$gte"] = t
			}
		}
		if p.To != "" {
			if t, e := time.Parse("2006-01-02", p.To); e == nil {
				r["$lt"] = t.Add(24 * time.Hour)
			}
		}
		if len(r) > 0 {
			filter["updatedAt"] = r
		}
	}
	if p.OnlyIds != "" {
		var ids []primitive.ObjectID
		for _, s := range strings.Split(p.OnlyIds, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if oid, err := primitive.ObjectIDFromHex(s); err == nil {
				ids = append(ids, oid)
			}
		}
		if len(ids) > 0 {
			filter["_id"] = bson.M{"$in": ids}
		} else {
			// ไม่มี id ที่ถูกต้องเลย — คืนผลลัพธ์ว่างเร็ว ๆ
			empty := &kwatmod.ExportResult{
				Bucket:   "",
				Key:      "",
				Size:     0,
				FileName: "",
				URL:      "",
			}
			return empty, nil
		}
	}

	// ---------- คิวรี่แบบสตรีม ----------
	watchlistColl := os.Getenv("WATCHLIST_COLLECTION")
	if watchlistColl == "" {
		watchlistColl = "kwatch_watchlist"
	}
	coll := config.DB.Collection(watchlistColl)

	findOpts := options.Find().
		SetSort(bson.D{{Key: "updatedAt", Value: -1}}).
		SetBatchSize(2000).
		SetNoCursorTimeout(true)
	if useLimit {
		findOpts = findOpts.SetLimit(p.Limit)
	}

	cur, err := coll.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	// เตรียม tmp
	tmp := filepath.Join(os.TempDir(), "export-"+jobID)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return nil, err
	}

	// 1) สร้าง Excel และดึงลิสต์งานรูป
	xlsx := filepath.Join(tmp, "watchlists.xlsx")
	tasks, err := buildExcel(ctx, cur, xlsx)
	if err != nil {
		return nil, err
	}

	// 2) ZIP + รูปจาก tasks
	zipPath := filepath.Join(tmp, "watchlists_"+time.Now().Format("20060102_150405")+".zip")
	if err := makeZipWithImages(ctx, xlsx, zipPath, tasks); err != nil {
		return nil, err
	}

	// 3) อัปโหลดขึ้น S3 (bucket document)
	targetBucket := os.Getenv("S3_BUCKET_DOCUMENT")
	if targetBucket == "" {
		targetBucket = "document"
	}
	key := fmt.Sprintf("exports/watchlists/%s/%s", jobID, filepath.Base(zipPath))

	fi, _ := os.Stat(zipPath)
	f, _ := os.Open(zipPath)
	defer f.Close()

	_, err = config.S3Client.PutObject(ctx, targetBucket, key, f, fi.Size(), config.S3PutOptionsContentZip())
	if err != nil {
		return nil, err
	}

	url := stos3minio.GetS3URL(targetBucket, key)

	return &kwatmod.ExportResult{
		Bucket:   targetBucket,
		Key:      key,
		Size:     fi.Size(),
		FileName: filepath.Base(zipPath),
		URL:      url,
	}, nil
}

// สร้างไฟล์ Excel (stream) และคืนรายการรูปที่ต้องดาวน์โหลด
func buildExcel(ctx context.Context, cur Cursor, out string) ([]imgTask, error) {
	type row struct {
		ID             primitive.ObjectID `bson:"_id"`
		PersonKey      string             `bson:"personKey"`
		FullName       string             `bson:"fullName"`
		Titlename      string             `bson:"titlename"`
		Sex            string             `bson:"sex"`
		Nation         string             `bson:"nation"`
		Source         string             `bson:"source"`
		CrimesType     int                `bson:"crimesType"`
		AlertDesc      string             `bson:"alertDesc"`
		UpdatedAt      time.Time          `bson:"updatedAt"`
		SeenAt         time.Time          `bson:"seenAt"`
		PhotoFaceKey   string             `bson:"photoFaceKey"`
		PhotoOriginKey string             `bson:"photoOriginKey"`
		Warrents       []struct {
			WarrantNo        string `bson:"warrantNo"`
			WarrantYear      string `bson:"warrantYear"`
			WarrantOrg       string `bson:"warrantOrg"`
			StatusWarrant    string `bson:"statusWarrant"`
			Charge           string `bson:"charge"`
			PoliceRegion     string `bson:"policeRegion"`
			PoliceProvincial string `bson:"policeProvincial"`
			PoliceStation    string `bson:"policeStation"`
		} `bson:"warrants"`
	}

	f := excelize.NewFile()
	sh := "Watchlists"
	f.SetSheetName("Sheet1", sh)
	sw, err := f.NewStreamWriter(sh)
	if err != nil {
		return nil, err
	}

	// Alert ไว้คอลัมน์สุดท้าย + มี BSON ID
	headers := []any{
		"No", "ID", "personalKey", "Full Name", "Title", "Sex", "Nation",
		"Source", "CrimesType",
		"PoliceRegion", "PoliceProvincial", "PoliceStation",
		"WarrantNo/Year", "WarrantOrg", "WarrantStatus",
		"FirstSeenAt", "UpdatedAt",
		"PhotoFaceKey", "PhotoOriginKey",
		"Alert", // last
	}
	if err := sw.SetRow("A1", headers); err != nil {
		return nil, err
	}

	var tasks []imgTask
	i, r := 1, 2
	for cur.Next(ctx) {
		var d row
		if e := cur.Decode(&d); e != nil {
			return nil, e
		}

		// join alert
		var alerts []string
		if s := strings.TrimSpace(d.AlertDesc); s != "" {
			alerts = append(alerts, s)
		}
		for idx, w := range d.Warrents {
			tag := fmt.Sprintf("หมายจับ #%d: %s/%s | ข้อหา: %s | ภูมิภาค: %s | จังหวัด: %s | สถานี: %s",
				idx+1, strings.TrimSpace(w.WarrantNo), strings.TrimSpace(w.WarrantYear),
				strings.TrimSpace(w.Charge),
				strings.TrimSpace(w.PoliceRegion), strings.TrimSpace(w.PoliceProvincial), strings.TrimSpace(w.PoliceStation),
			)
			alerts = append(alerts, tag)
		}
		alertOut := strings.Join(alerts, ", ")

		// choose first warrant for police/org columns
		var pr, pp, ps, won, worg, wst string
		if len(d.Warrents) > 0 {
			w := d.Warrents[0]
			if w.WarrantNo != "" || w.WarrantYear != "" {
				won = strings.TrimSpace(w.WarrantNo + "/" + w.WarrantYear)
			}
			pr, pp, ps, worg, wst = w.PoliceRegion, w.PoliceProvincial, w.PoliceStation, w.WarrantOrg, w.StatusWarrant
		}

		vals := []any{
			i,
			d.ID.Hex(), // BSON_ID
			d.PersonKey, d.FullName, d.Titlename, d.Sex, d.Nation,
			d.Source, d.CrimesType,
			pr, pp, ps,
			won, worg, wst,
			d.SeenAt.Local().Format("2006-01-02 15:04:05"),
			d.UpdatedAt.Local().Format("2006-01-02 15:04:05"),
			d.PhotoFaceKey, d.PhotoOriginKey,
			alertOut,
		}
		cell, _ := excelize.CoordinatesToCellName(1, r)
		if err := sw.SetRow(cell, vals); err != nil {
			return nil, err
		}

		// เตรียมรายการดาวน์โหลดรูป (สร้างโฟลเดอร์ images/<_id>/)
		if strings.TrimSpace(d.PhotoFaceKey) != "" || strings.TrimSpace(d.PhotoOriginKey) != "" {
			t := imgTask{
				DocID:     d.ID.Hex(),
				FaceKey:   d.PhotoFaceKey,
				OriginKey: d.PhotoOriginKey,
				FaceExt:   fileExtFromKey(d.PhotoFaceKey),
				OriginExt: fileExtFromKey(d.PhotoOriginKey),
			}
			tasks = append(tasks, t)
		}

		i++
		r++
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	if err := sw.Flush(); err != nil {
		return nil, err
	}
	if err := f.SaveAs(out); err != nil {
		return nil, err
	}
	return tasks, nil
}

// ทำ ZIP + แนบรูปตาม tasks
func makeZipWithImages(ctx context.Context, xlsxPath, zipPath string, tasks []imgTask) error {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kwatsvc", "Zip", "kwatsvc", "zip")
	defer end()

	bucketKwatch := os.Getenv("S3_BUCKET_KWATCH")
	if bucketKwatch == "" {
		bucketKwatch = "kwatch"
	}

	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)

	// 1) add excel
	{
		w, _ := zw.Create("watchlists.xlsx")
		r, err := os.Open(xlsxPath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, r); err != nil {
			r.Close()
			return err
		}
		r.Close()
	}

	// 2) add images/<_id>/(face.ext, origin.ext)
	sem := make(chan struct{}, 8)
	errCh := make(chan error, len(tasks))
	for _, t := range tasks {
		t := t
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			baseDir := "images/" + t.DocID

			// face
			if strings.TrimSpace(t.FaceKey) != "" {
				if data, _, e := stos3minio.DownloadByKey(ctx, bucketKwatch, t.FaceKey); e != nil {
					log.Warn().Err(e).Str("key", t.FaceKey).Msg("skip face")
				} else {
					name := "face" + safeExt(t.FaceExt, ".jpg")
					w, e := zw.Create(filepath.Join(baseDir, name))
					if e != nil {
						errCh <- e
						return
					}
					if _, e := w.Write(data); e != nil {
						errCh <- e
						return
					}
				}
			}
			// origin
			if strings.TrimSpace(t.OriginKey) != "" {
				if data, _, e := stos3minio.DownloadByKey(ctx, bucketKwatch, t.OriginKey); e != nil {
					log.Warn().Err(e).Str("key", t.OriginKey).Msg("skip origin")
				} else {
					name := "origin" + safeExt(t.OriginExt, ".jpg")
					w, e := zw.Create(filepath.Join(baseDir, name))
					if e != nil {
						errCh <- e
						return
					}
					if _, e := w.Write(data); e != nil {
						errCh <- e
						return
					}
				}
			}
		}()
	}
	// wait
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
	close(errCh)
	for e := range errCh {
		if e != nil {
			_ = zw.Close()
			return e
		}
	}
	return zw.Close()
}

// ======================= Helpers =======================

func fileExtFromKey(key string) string {
	if key == "" {
		return ""
	}
	ext := filepath.Ext(key)
	return strings.ToLower(ext)
}

func safeExt(ext, def string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return ext
	default:
		return def
	}
}

// mongo cursor interface (testing friendly)
type Cursor interface {
	Next(context.Context) bool
	Decode(any) error
	Err() error
	Close(context.Context) error
}
