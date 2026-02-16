// internal/services/crimes/watch.go
package crimes

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	intlog "github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/fsnotify/fsnotify"
	"go.mongodb.org/mongo-driver/mongo"
)

// เพิ่ม import เพิ่มเติม (ถ้ายังไม่มี)
// "regexp"

var (
	// ไฟล์ที่เราสนใจ (คุณปรับ regex ให้ตรง pattern จริงของคุณได้)
	// ตัวอย่างนี้รองรับทั้ง *_warrant_crimes.csv และ *_warrant_crimeslittle.csv
	csvNameRegex = regexp.MustCompile(`(?i).*_warrant_crimes.*\.csv$`)
	pollInterval = 5 * time.Second
)

// สแกนโฟลเดอร์รอบเดียว: คืนรายชื่อไฟล์ที่ "น่าจะใหม่/ยังไม่ได้ประมวลผล"
func scanDirOnce(dir string, processed map[string]struct{}) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// ข้าม temp ขณะอัปโหลด
		if strings.HasSuffix(name, ".filepart") || strings.HasPrefix(name, ".") {
			continue
		}
		if !csvNameRegex.MatchString(name) {
			continue
		}
		full := filepath.Join(dir, name)
		if _, seen := processed[full]; seen {
			continue
		}
		paths = append(paths, full)
	}
	return paths, nil
}

// รอจนขนาดไฟล์คงที่ (กันอ่านไฟล์ระหว่างอัปโหลด)
func waitForComplete(path string, timeout time.Duration) error {
	var prev int64 = -1
	deadline := time.Now().Add(timeout)

	for {
		fi, err := os.Stat(path)
		if err != nil {
			return err
		}
		size := fi.Size()
		if size > 0 && size == prev {
			return nil // stable
		}
		prev = size
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s", path)
		}
		time.Sleep(1 * time.Second)
	}
}

// ย้ายไฟล์ (รองรับข้าม filesystem)
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return os.Remove(src)
}

const (
	tmpDir = "/tmp"
	// เปลี่ยนเป็น "*.csv" หากต้องการลบ .csv ทั้งหมดใน /tmp
	tmpCSVPattern = "*.csv"
	csvStableWait = 30 * time.Second
)

// ลบไฟล์ .csv เก่าตามแพทเทิร์นใน /tmp ให้เหลือเวอร์ชันล่าสุดไว้ตัวเดียว
// ใช้ logger.FromCtx เพื่อผูก traceId/spanId ลง log
func cleanupTmpCSVs(ctx context.Context, pattern string) int {
	log := intlog.FromCtx(ctx, "crimes", "cleanupTmpCSVs")

	matches, err := filepath.Glob(filepath.Join(tmpDir, pattern))
	if err != nil {
		log.Warn().Err(err).Str("dir", tmpDir).Str("pattern", pattern).Msg("glob tmp csv failed")
		return 0
	}
	deleted := 0
	for _, f := range matches {
		if err := os.Remove(f); err != nil {
			log.Warn().Err(err).Str("file", f).Msg("remove old csv failed")
			continue
		}
		deleted++
	}
	if deleted > 0 {
		log.Info().Int("deleted", deleted).Str("pattern", pattern).Str("dir", tmpDir).Msg("🧹 removed old csv(s) from tmp")
	}
	return deleted
}

func WatchCrimesDir(ctx context.Context, dir string, coll *mongo.Collection) error {
	ctx, end, _ := traceutil.StartLite(
		ctx, "github.com/hotkhwan/gateway-api/crimes", "crimes.WatchCrimesDir", "crimes", "WatchCrimesDir",
	)
	defer end()
	log := intlog.FromCtx(ctx, "crimes", "WatchCrimesDir")

	// dedupe: กันประมวลผลซ้ำ
	processed := make(map[string]struct{})
	doneCh := make(chan string, 256)

	// --- watcher ส่วนเดิม (จะทำงานเฉพาะบน local FS/RWO; บน NFS มักไม่ยิง event) ---
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error().Err(err).Msg("create watcher failed")
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(dir); err != nil {
		// ไม่ fail ทั้งโปรแกรม — แค่แจ้งเตือน แล้วให้ polling ทำงานแทน
		log.Warn().Err(err).Str("dir", dir).Msg("watcher add failed; fallback to polling only")
	}

	log.Info().Str("dir", dir).Msg("👀 Watching crimes directory (with polling fallback)")

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	handleOne := func(parentCtx context.Context, path string) {
		// mark processed ที่ loop หลักเท่านั้น (ปลอดภัย)
		if _, seen := processed[path]; seen {
			return
		}
		processed[path] = struct{}{}

		go func(parentCtx context.Context, path string) {
			childCtx, childEnd, _ := traceutil.StartLite(
				parentCtx, "github.com/hotkhwan/gateway-api/crimes", "crimes.HandleNewFile",
				"crimes", "HandleNewFile",
			)
			defer childEnd()
			childLog := intlog.FromCtx(childCtx, "crimes", "HandleNewFile")

			if err := waitForComplete(path, csvStableWait); err != nil {
				childLog.Warn().Err(err).Str("file", path).Msg("file not ready")
				// ❗️ขอให้ loop หลักลบ key เอง
				doneCh <- path
				return
			}

			if err := ImportCrimesFile(childCtx, path, coll); err != nil {
				childLog.Error().Err(err).Str("file", path).Msg("import error")
				// ❗️ลบ key เพื่อให้ polling ลองใหม่รอบหน้า
				doneCh <- path
				return
			}

			cleanupTmpCSVs(childCtx, tmpCSVPattern)
			removed := cleanupTmpCSVs(childCtx, tmpCSVPattern)
			if removed > 0 {
				childLog.Info().Int("deleted", removed).Msg("🧹 cleared tmp CSVs before move")
			}
			newPath := filepath.Join(tmpDir, filepath.Base(path))
			if err := moveFile(path, newPath); err != nil {
				childLog.Error().Err(err).Str("src", path).Str("dst", newPath).Msg("move file failed")
				// ถ้าย้ายไม่ได้ ให้ลบ key เพื่อให้ลองใหม่
				doneCh <- path
				return
			}

			childLog.Info().Str("dst", newPath).Msg("📦 moved file")
			// ✅ สำเร็จแล้ว: ลบ key เพื่อ “ปล่อยชื่อเดิม” ใช้ได้ในอนาคต
			doneCh <- path
		}(parentCtx, path)
	}

	for {
		select {
		case ev := <-watcher.Events:
			// ... (เหมือนเดิม)
			handleOne(ctx, ev.Name)

		case err := <-watcher.Errors:
			log.Warn().Err(err).Msg("watcher error")

		case <-ticker.C:
			list, err := scanDirOnce(dir, processed)
			if err != nil {
				log.Warn().Err(err).Msg("poll scan failed")
				continue
			}
			for _, p := range list {
				log.Info().Str("file", p).Msg("🔎 polling detected new file")
				handleOne(ctx, p)
			}

		// ✅ ลบ key ที่ goroutine ลูกส่งสัญญาณมา (ป้องกัน data race)
		case p := <-doneCh:
			delete(processed, p)

		case <-ctx.Done():
			log.Info().Msg("🛑 stopping crimes watcher")
			return nil
		}
	}
}
