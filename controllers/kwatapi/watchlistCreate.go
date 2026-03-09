// controllers/kwatapi/watchlistCreate.go
package kwatapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/services/kwatsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kwatmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

var validate = validator.New()

// watchlistCreate godoc
// @Summary Create a new watchlist
// @Description สร้างรายการเฝ้าระวัง (watchlist) พร้อมภาพจาก **ไฟล์อัปโหลด** หรือ **ลิงก์ URL** (ต้องมีอย่างใดอย่างหนึ่ง)
// @Tags Kwatch/Watchlist
// @Accept multipart/form-data
// @Produce json
// @Param type formData string true "ประเภทบุคคล"
// @Param personalType formData string false "ประเภทบุคคลส่วนตัว"
// @Param crimesType formData string false "ประเภทคดี"
// @Param idcard formData string false "บัตรประชาชน"
// @Param passport formData string false "พาสปอร์ต"
// @Param titlename formData string false "คำนำหน้า"
// @Param subTitlename formData string false "คำนำหน้ารอง"
// @Param firstname formData string true "ชื่อจริง"
// @Param lastname formData string true "นามสกุล"
// @Param nickname formData string false "ชื่อเล่น"
// @Param sex formData string false "เพศ"
// @Param birthday formData string false "วันเกิด"
// @Param age formData string false "อายุ"
// @Param fatherName formData string false "ชื่อบิดา"
// @Param fatherIdcard formData string false "บัตรประชาชนบิดา"
// @Param motherName formData string false "ชื่อมารดา"
// @Param motherIdcard formData string false "บัตรประชาชนมารดา"
// @Param maritalStatus formData string false "สถานภาพสมรส"
// @Param deathStatus formData string false "สถานะการเสียชีวิต"
// @Param dateOfDeath formData string false "วันที่เสียชีวิต"
// @Param policeRegion formData string false "ภาคตำรวจ"
// @Param policeProvincial formData string false "ตำรวจภูธรจังหวัด"
// @Param policeStation formData string false "สถานีตำรวจ"
// @Param userRecorder formData string false "ผู้บันทึกข้อมูล"
// @Param userPosition formData string false "ตำแหน่งผู้บันทึก"
// @Param status formData string false "สถานะ"
// @Param state formData string false "State"
// @Param alertType formData string false "ประเภทการแจ้งเตือน"
// @Param alertDesc formData string false "รายละเอียดการแจ้งเตือน"
// @Param photo formData file false "ไฟล์รูปภาพ (ถ้ามี photoUrl ไม่จำเป็นต้องส่ง)"
// @Param photoUrl formData string false "ลิงก์รูปภาพ (ถ้ามี photo ไม่จำเป็นต้องส่ง)"
// @Success 202 {object} gmod.SuccessMessageCreateResponse
// @Failure 400 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.ErrorMessageResponse
// @Router /watchlist [post]
// @Security BearerAuth
func WatchlistCreate(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.kwatapi", "Watchlist.WatchlistCreate", "kwatapi", "WatchlistCreate")
	defer end()

	var req kwatmod.WatchlistCreateRequest

	// ✅ mapping ฟิลด์จาก form
	req.Type = c.FormValue("type")
	req.PersonalType = c.FormValue("personalType")
	req.CrimesType = c.FormValue("crimesType")
	req.IdCard = c.FormValue("idcard")
	req.Passport = c.FormValue("passport")
	req.TitleName = c.FormValue("titlename")
	req.SubTitleName = c.FormValue("subTitlename")
	req.FirstName = c.FormValue("firstname")
	req.LastName = c.FormValue("lastname")
	req.NickName = c.FormValue("nickname")
	req.Sex = c.FormValue("sex")
	req.Birthday = c.FormValue("birthday")
	req.Age = c.FormValue("age")
	req.FatherName = c.FormValue("fatherName")
	req.FatherIdCard = c.FormValue("fatherIdcard")
	req.MotherName = c.FormValue("motherName")
	req.MotherIdCard = c.FormValue("motherIdcard")
	req.MaritalStatus = c.FormValue("maritalStatus")
	req.DeathStatus = c.FormValue("deathStatus")
	req.DateOfDeath = c.FormValue("dateOfDeath")
	req.PoliceRegion = c.FormValue("policeRegion")
	req.PoliceProvincial = c.FormValue("policeProvincial")
	req.PoliceStation = c.FormValue("policeStation")
	req.UserRecorder = c.FormValue("userRecorder")
	req.UserPosition = c.FormValue("userPosition")
	req.Status = c.FormValue("status")
	req.AlertType = c.FormValue("alertType")
	req.AlertDesc = c.FormValue("alertDesc")

	// ✅ รับค่า photoUrl
	req.PhotoURL = strings.TrimSpace(c.FormValue("photoUrl"))

	// ✅ อย่างน้อยต้องมีรูปจากไฟล์หรือจาก URL อย่างใดอย่างหนึ่ง
	file, fileErr := c.FormFile("photo")
	if (fileErr != nil || file == nil) && req.PhotoURL == "" {
		return httputil.FailBadRequest(c, "PHOTO_REQUIRED",
			"Specify the image via file (photo) or URL (photoUrl), crush one.")
	}

	// --- กรณี 1: แนบไฟล์มา → ใช้ไฟล์เป็นหลัก ---
	if fileErr == nil && file != nil {
		openedFile, err := file.Open()
		if err != nil {
			return httputil.FailBadRequest(c, "BAD_PHOTO_FILE", "cannot open photo file")
		}
		defer openedFile.Close()

		fileBytes, err := io.ReadAll(openedFile)
		if err != nil || len(fileBytes) == 0 {
			return httputil.FailBadRequest(c, "BAD_PHOTO_FILE", "photo file is empty")
		}
		req.PhotoFile = fileBytes
		req.PhotoFileName = file.Filename
		req.PhotoContentType = file.Header.Get("Content-Type")
		req.PhotoSource = "upload"
	} else {
		// --- กรณี 2: ไม่มีไฟล์ แต่มี photoUrl → ดาวน์โหลด ---
		if err := fetchImageFromURL(&req); err != nil {
			// map เป็น 400 เพื่อบอกว่ารูปจาก URL ใช้งานไม่ได้
			return httputil.FailBadRequest(c, "BAD_PHOTO_URL", err.Error())
		}
		req.PhotoSource = "url"
	}

	// ✅ ตรวจชื่อ/นามสกุลที่ต้องมี
	if req.FirstName == "" {
		return httputil.FailBadRequest(c, "field `firstname` is required")
	}
	if req.LastName == "" {
		return httputil.FailBadRequest(c, "field `lastname` is required")
	}

	// ✅ Validation เสริม (หากมี tag validate ใน struct)
	if err := validate.Struct(&req); err != nil {
		return err
	}

	log.Info().
		Str("idcard", req.IdCard).
		Int("photoBytes", len(req.PhotoFile)).
		Str("photoSource", req.PhotoSource).
		Msg("api_request_in")

	// ✅ เรียก service
	id, err := kwatsvc.WatchlistCreate(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, kwatsvc.ErrDuplicateIDCard):
			return httputil.FailBadRequest(c, "DUPLICATE_IDCARD", "ID card duplicate")
		case errors.Is(err, kwatsvc.ErrNoFace):
			return httputil.FailBadRequest(c, "BAD_PHOTO_NO_FACE", "Photo does not contain a detectable face")
		case errors.Is(err, kwatsvc.ErrBadPhoto):
			return httputil.FailBadRequest(c, "BAD_PHOTO", "Photo is damaged or invalid")
		case errors.Is(err, kwatsvc.ErrPhotoRequired):
			return httputil.FailBadRequest(c, "PHOTO_REQUIRED", "Photo file is required")
		default:
			return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to create watchlist")
		}
	}

	log.Info().Str("docId", id.Hex()).Msg("api_request_success")
	return gmod.SendAcceptedWithID(c, "SUCCESS", "Watchlist created successfully", id.Hex())
}

// --- Helper: ดึงรูปจาก URL อย่างปลอดภัย พร้อมเดา content-type/ชื่อไฟล์ ---
func fetchImageFromURL(req *kwatmod.WatchlistCreateRequest) error {
	const maxBytes int64 = 15 * 1024 * 1024 // 15MB
	u := req.PhotoURL
	if u == "" {
		return errors.New("photoUrl is empty")
	}

	// ตรวจ URL คร่าว ๆ
	if _, err := url.ParseRequestURI(u); err != nil {
		return errors.New("invalid photoUrl")
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Get(u)
	if err != nil {
		return errors.New("cannot fetch photo from URL")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("photoUrl response is not OK")
	}

	// จำกัดขนาด
	lr := io.LimitReader(resp.Body, maxBytes+1)
	body, err := io.ReadAll(lr)
	if err != nil {
		return errors.New("cannot read photo from URL")
	}
	if int64(len(body)) > maxBytes {
		return errors.New("photo from URL exceeds max size 15MB")
	}
	if len(body) == 0 {
		return errors.New("photo from URL is empty")
	}

	// เดา content-type จาก header ถ้าไม่มี/ไม่ชัดเจน ใช้ sniff แบบง่าย
	ct := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if ct == "" || !strings.HasPrefix(ct, "image/") {
		ct = sniffContentType(body) // image/jpeg | image/png เป็นต้น
	}
	if ct == "" || !strings.HasPrefix(ct, "image/") {
		return errors.New("photoUrl is not an image")
	}

	// ตั้งชื่อไฟล์จาก path ท้ายสุดของ URL ถ้าไม่มี ใช้ชื่อ default ตามนามสกุลจาก mime
	filename := filenameFromURL(u, ct)

	req.PhotoFile = body
	req.PhotoFileName = filename
	req.PhotoContentType = ct
	return nil
}

func sniffContentType(b []byte) string {
	// ใช้ http.DetectContentType พอเพียงสำหรับรูปทั่วไป
	ct := http.DetectContentType(b)
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	// เผื่อบางเคสตอบเป็น "application/octet-stream" ให้เดาเป็น jpg
	return "image/jpeg"
}

func filenameFromURL(u string, contentType string) string {
	parsed, err := url.Parse(u)
	if err == nil {
		base := path.Base(parsed.Path)
		if base != "" && base != "/" && !strings.HasSuffix(base, "/") {
			// ถ้าไม่มีนามสกุล ลองเติมจาก mime
			if !strings.Contains(base, ".") {
				if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
					return base + exts[0]
				}
			}
			return base
		}
	}
	// default name ตาม mime
	ext := ".jpg"
	if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
		ext = exts[0]
	}
	return "photo" + ext
}

// package kwatapi

// import (
// 	"errors"
// 	"io"
// 	"github.com/hotkhwan/gateway-api/internal/logger"
// 	"github.com/hotkhwan/gateway-api/internal/services/kwatsvc"
// 	"github.com/hotkhwan/gateway-api/models/gmod"
// 	"github.com/hotkhwan/gateway-api/models/kwatmod"
// 	"github.com/hotkhwan/gateway-api/utils/traceutil"

// 	"github.com/go-playground/validator/v10"
// 	"github.com/gofiber/fiber/v2"
// 	"go.opentelemetry.io/otel"
// 	"go.opentelemetry.io/otel/trace"
// )

// var validate = validator.New()

// // watchlistCreate godoc
// // @Summary Create a new watchlist
// // @Description สร้างรายการเฝ้าระวัง (watchlist) พร้อมอัปโหลดไฟล์รูปภาพ
// // @Tags Kwatch/Watchlist
// // @Accept multipart/form-data
// // @Produce json
// // @Param type formData string true "ประเภทบุคคล"
// // @Param personalType formData string false "ประเภทบุคคลส่วนตัว"
// // @Param crimesType formData string false "ประเภทคดี"
// // @Param idcard formData string false "บัตรประชาชน"
// // @Param passport formData string false "พาสปอร์ต"
// // @Param titlename formData string false "คำนำหน้า"
// // @Param subTitlename formData string false "คำนำหน้ารอง"
// // @Param firstname formData string true "ชื่อจริง"
// // @Param lastname formData string true "นามสกุล"
// // @Param nickname formData string false "ชื่อเล่น"
// // @Param sex formData string false "เพศ"
// // @Param birthday formData string false "วันเกิด"
// // @Param age formData string false "อายุ"
// // @Param fatherName formData string false "ชื่อบิดา"
// // @Param fatherIdcard formData string false "บัตรประชาชนบิดา"
// // @Param motherName formData string false "ชื่อมารดา"
// // @Param motherIdcard formData string false "บัตรประชาชนมารดา"
// // @Param maritalStatus formData string false "สถานภาพสมรส"
// // @Param deathStatus formData string false "สถานะการเสียชีวิต"
// // @Param dateOfDeath formData string false "วันที่เสียชีวิต"
// // @Param policeRegion formData string false "ภาคตำรวจ"
// // @Param policeProvincial formData string false "ตำรวจภูธรจังหวัด"
// // @Param policeStation formData string false "สถานีตำรวจ"
// // @Param userRecorder formData string false "ผู้บันทึกข้อมูล"
// // @Param userPosition formData string false "ตำแหน่งผู้บันทึก"
// // @Param status formData string false "สถานะ"
// // @Param state formData string false "State"
// // @Param photo formData file true "ไฟล์รูปภาพ"
// // @Success 202 {object} gmod.SuccessMessageCreateResponse
// // @Failure 400 {object} gmod.ErrorMessageResponse
// // @Failure 500 {object} gmod.ErrorMessageResponse
// // @Router /watchlist [post]
// // @Security BearerAuth
// func WatchlistCreate(c *fiber.Ctx) error {
// 	ctx := c.UserContext()
// 	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/kwatapi")
// 	ctx, span := tracer.Start(ctx, "Kwatch.WatchlistCreate")
// 	defer span.End()

// 	// ✅ ผูก traceID (ให้ logger ที่ยังไม่อ่าน OTel เห็นได้)
// 	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
// 		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
// 	}

// 	// ✅ logger จาก ctx + ตั้งชื่อ component/source ให้คงที่ทั้งโปรเจกต์
// 	log := logger.FromCtx(ctx, "kwatapi", "WatchlistCreate")
// 	var req kwatmod.WatchlistCreateRequest

// 	// ✅ ใช้ c.FormValue กับ struct mapping แบบ manual เพราะ multipart/form-data
// 	req.Type = c.FormValue("type")
// 	req.PersonalType = c.FormValue("personalType")
// 	req.CrimesType = c.FormValue("crimesType")
// 	req.IdCard = c.FormValue("idcard")
// 	req.Passport = c.FormValue("passport")
// 	req.TitleName = c.FormValue("titlename")
// 	req.SubTitleName = c.FormValue("subTitlename")
// 	req.FirstName = c.FormValue("firstname")
// 	req.LastName = c.FormValue("lastname")
// 	req.NickName = c.FormValue("nickname")
// 	req.Sex = c.FormValue("sex")
// 	req.Birthday = c.FormValue("birthday")
// 	req.Age = c.FormValue("age")
// 	req.FatherName = c.FormValue("fatherName")
// 	req.FatherIdCard = c.FormValue("fatherIdcard")
// 	req.MotherName = c.FormValue("motherName")
// 	req.MotherIdCard = c.FormValue("motherIdcard")
// 	req.MaritalStatus = c.FormValue("maritalStatus")
// 	req.DeathStatus = c.FormValue("deathStatus")
// 	req.DateOfDeath = c.FormValue("dateOfDeath")
// 	req.PoliceRegion = c.FormValue("policeRegion")
// 	req.PoliceProvincial = c.FormValue("policeProvincial")
// 	req.PoliceStation = c.FormValue("policeStation")
// 	req.UserRecorder = c.FormValue("userRecorder")
// 	req.UserPosition = c.FormValue("userPosition")
// 	req.Status = c.FormValue("status")
// 	req.AlertType = c.FormValue("alertType")
// 	req.AlertDesc = c.FormValue("alertDesc")

// 	log.Info().Str("idcard", req.IdCard).Int("photoBytes", len(req.PhotoFile)).Msg("api_request_in")

// 	// ✅ ตรวจสอบว่า photo file ต้องมีจริง และไม่ว่าง
// 	file, err := c.FormFile("photo")
// 	if err != nil || file == nil {
// 		return fiber.NewError(fiber.StatusBadRequest, "field `photo` is required")
// 	}

// 	openedFile, err := file.Open()
// 	if err != nil {
// 		return fiber.NewError(fiber.StatusBadRequest, "cannot open photo file")
// 	}
// 	defer openedFile.Close()

// 	fileBytes, err := io.ReadAll(openedFile)
// 	if err != nil || len(fileBytes) == 0 {
// 		return fiber.NewError(fiber.StatusBadRequest, "photo file is empty")
// 	}

// 	req.PhotoFile = fileBytes
// 	req.PhotoFileName = file.Filename
// 	req.PhotoContentType = file.Header.Get("Content-Type")

// 	// ✅ ตรวจสอบฟิลด์อื่น ๆ ที่ต้องมี
// 	if req.FirstName == "" {
// 		log.Error().Err(err).Msg("❌ field `firstname` is required")
// 		return fiber.NewError(fiber.StatusBadRequest, "field `firstname` is required")
// 	}
// 	if req.LastName == "" {
// 		return fiber.NewError(fiber.StatusBadRequest, "field `lastname` is required")
// 	}

// 	// ✅ Validation (struct field level)
// 	if err := validate.Struct(&req); err != nil {
// 		return err
// 	}

// 	// ✅ Service Logic
// 	id, err := kwatsvc.WatchlistCreate(ctx, req)
// 	if err != nil {
// 		switch {
// 		case errors.Is(err, kwatsvc.ErrDuplicateIDCard):
// 			return gmod.SendError(c, fiber.StatusBadRequest, "DUPLICATE_IDCARD", "ID card duplicate")
// 		case errors.Is(err, kwatsvc.ErrNoFace):
// 			return gmod.SendError(c, fiber.StatusBadRequest, "BAD_PHOTO_NO_FACE", "Photo does not contain a detectable face")
// 		case errors.Is(err, kwatsvc.ErrBadPhoto):
// 			return gmod.SendError(c, fiber.StatusBadRequest, "BAD_PHOTO", "Photo is damaged or invalid")
// 		case errors.Is(err, kwatsvc.ErrPhotoRequired):
// 			return gmod.SendError(c, fiber.StatusBadRequest, "PHOTO_REQUIRED", "Photo file is required")
// 		default:
// 			return gmod.SendError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create watchlist")
// 		}
// 	}
// 	log.Info().Str("docId", id.Hex()).Msg("api_request_success")
// 	return gmod.SendAcceptedWithID(c, "SUCCESS", "Watchlist created successfully", id.Hex())
// }
