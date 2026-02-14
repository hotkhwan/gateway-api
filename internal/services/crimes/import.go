// internal/services/crimes/import.go
package crimes

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"klynx/models"
	"klynx/utils/traceutil" // ✅ ใช้ StartLite

	"go.mongodb.org/mongo-driver/mongo"
)

func cleanHeader(h string) string {
	h = strings.TrimPrefix(h, "\uFEFF")
	h = strings.TrimPrefix(h, "\ufeff")
	h = strings.TrimSpace(h)
	h = strings.Trim(h, "\"")
	return h
}

// --- CSV Importer (aggregate to persons) -------------------------------------
func ImportCrimesFile(ctx context.Context, path string, coll *mongo.Collection) error {
	// ✅ start span + logger
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/crimes",            // tracerName
		"crimes.ImportCrimesFile", // spanName
		"crimes", "ImportCrimesFile",
	)
	defer end()

	raw, err := os.ReadFile(path)
	if err != nil {
		log.Error().Err(err).Str("file", path).Msg("read file error")
		return fmt.Errorf("read file: %w", err)
	}
	if len(raw) == 0 {
		log.Warn().Str("file", path).Msg("empty csv file")
		return fmt.Errorf("empty file: %s", path)
	}

	log.Info().
		Str("file", path).
		Int("size", len(raw)).
		Msg("📄 Import crimes CSV")

	r := csv.NewReader(decodeToUTF8(raw))
	r.Comma = ','
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	headers, err := r.Read()
	if err != nil {
		log.Error().Err(err).Msg("read header")
		return fmt.Errorf("read header: %w", err)
	}
	h := map[string]int{}
	for i, v := range headers {
		h[cleanHeader(v)] = i
	}

	// counters
	totalRows := 0
	usable := 0
	missingKey := 0
	badIdLen := 0

	// group by personKey
	persons := map[string]*models.CrimeImport{}
	dupRows := map[string][]int{} // key -> row indices (1-based)
	rowIdx := 1                   // header row = 1

	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		rowIdx++
		totalRows++
		if err != nil {
			log.Warn().Int("row", rowIdx).Err(err).Msg("skip row: read error")
			continue
		}

		get := func(name string) string {
			if idx, ok := h[name]; ok && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		idNo := get("ID_NO")
		passport := get("PASSPORT_NO")

		// keep only digits in ID card
		idNo = strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, idNo)

		if idNo != "" && len(idNo) != 13 {
			badIdLen++
		}

		key := idNo
		if key == "" {
			key = passport
		}
		if key == "" {
			missingKey++
			continue
		}

		usable++
		dupRows[key] = append(dupRows[key], rowIdx)

		p, ok := persons[key]
		if !ok {
			p = &models.CrimeImport{
				IdNo:         idNo,
				PassportNo:   passport,
				PersonKey:    key,
				CrimeId:      get("ID"),
				FullName:     get("FULL_NAME"),
				TitleString:  get("TITLESTRING"),
				FirstName:    get("FIRST_NAME"),
				LastName:     get("LAST_NAME"),
				TitleEn:      get("TITLE_EN"),
				FirstNameEn:  get("FIRST_NAME_EN"),
				MiddleNameEn: get("MIDDLE_NAME_EN"),
				LastNameEn:   get("LAST_NAME_EN"),
				Sex:          get("GENDER"),
				Birthday:     parseThaiDate(get("BIRTHDAY")),
				Nation:       get("NATION"),
				Country:      get("COUNTRY"),
				ImportId:     get("ID"),
				Source:       "crimes",
			}
			persons[key] = p
		}

		p.Warrants = append(p.Warrants, models.Warrant{
			WarrantNo:        get("WARRANT_NO"),
			WarrantYear:      get("WARRANT_YEAR"),
			WarrantOrgCode:   get("WARRANT_ORG_CODE"),
			WarrantOrg:       get("WARRANT_ORG"),
			CrimeNo:          get("CRIME_NO"),
			CrimeYear:        get("CRIME_YEAR"),
			Charge:           get("CHARGE"),
			WarrantDate:      parseThaiDate(get("WARRANT_DATE")),
			WarrantDueDate:   parseThaiDate(get("WARRANT_DUE_DATE")),
			WarrantLimitYear: get("WARRANT_LIMIT_YEAR"),
			Police:           get("POLICE"),
			PolisPos:         get("POLIS_POS"),
			PoliceStation:    get("ORG1"),
			PoliceRegion:     get("BH"),
			PoliceProvincial: get("BK"),
			BhCode:           get("BH_CODE"),
			BkCode:           get("BK_CODE"),
			OrgCode:          get("ORGCODE"),
			CrimeLocation:    get("CRIMELOCATION"),
			StatusWarrant:    get("STATUSWARRANT"),
		})
	}

	// pack to slice
	rows := make([]models.CrimeImport, 0, len(persons))
	keys := make([]string, 0, len(persons))
	for k, v := range persons {
		rows = append(rows, *v)
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// duplicates inside file
	dupsWhere := make([]string, 0)
	dupKeys := 0
	dupRowsExtra := 0
	for k, list := range dupRows {
		if len(list) > 1 {
			dupKeys++
			dupRowsExtra += len(list) - 1
			dupsWhere = append(dupsWhere, fmt.Sprintf("%s (rows: %v)", k, list))
		}
	}

	uniquePersons := len(persons)
	dupIdWithWarrant := usable - uniquePersons // = แถวส่วนเกินเพราะคนเดียวหลายหมายจับ

	// ---- เข้าฐาน (DB summary) ----
	dbSummary, err := SyncCrimesToWatchlist(ctx, rows, keys, dupsWhere, coll)
	if err != nil {
		log.Error().Err(err).Msg("sync to mongodb failed")
		return err
	}

	// ---- LOG แบบครบ (ลง trace + console) ----
	// ตรวจความสอดคล้อง
	mark := ""
	if dupIdWithWarrant != dupRowsExtra {
		mark = fmt.Sprintf(" (warn: dupRowsExtra=%d)", dupRowsExtra)
	}

	log.Info().
		Int("fileRows", totalRows).
		Int("usable", usable).
		Int("missingKey", missingKey).
		Int("badIdLen", badIdLen).
		Int("uniquePersons", uniquePersons).
		Int("dupIdWithWarrant", dupIdWithWarrant).
		Int("dupKeys", dupKeys).
		Msg("📊 Crimes file summary" + mark)

	log.Info().
		Int64("inserted", dbSummary.Inserted).
		Int64("updated", dbSummary.Updated).
		Int64("deleted", dbSummary.Deleted).
		Msg("🗃️ MongoDB write summary")

	if dupKeys > 0 {
		sample := dupsWhere
		if len(sample) > 20 {
			sample = sample[:20]
		}
		log.Info().
			Int("dupKeys", dupKeys).
			Strs("samples", sample).
			Msg("🔁 duplicate keys inside file")
	}

	return nil
}
