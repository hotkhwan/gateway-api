// controllers/kwatapi/watchlistUpdate.go
package kwatapi

import (
	"errors"
	"github.com/hotkhwan/gateway-api/internal/services/kwatsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kwatmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"io"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

func toPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// UpdatehWatchlist godoc
// @Summary Update an existing watchlist
// @Description อัปเดตรายการเฝ้าระวังตาม ID **หรือ** ID Card สามารถส่งข้อมูลเป็น JSON หรือ multipart/form-data พร้อมไฟล์รูปภาพได้
// @Tags Kwatch/Watchlist
// @Accept  json
// @Accept  multipart/form-data
// @Produce  json
// @Param   id      path      string  true  "Watchlist _id หรือ idcard"
// @Param   type formData string false "ประเภทบุคคล"
// @Param   personalType formData string false "ประเภทบุคคลส่วนตัว"
// @Param   crimesType formData string false "ประเภทคดี"
// @Param   idcard formData string false "บัตรประชาชน"
// @Param   passport formData string false "พาสปอร์ต"
// @Param   titlename formData string false "คำนำหน้า"
// @Param   subTitlename formData string false "คำนำหน้ารอง"
// @Param   firstname formData string false "ชื่อจริง"
// @Param   lastname formData string false "นามสกุล"
// @Param   nickname formData string false "ชื่อเล่น"
// @Param   sex formData string false "เพศ"
// @Param   birthday formData string false "วันเกิด"
// @Param   age formData string false "อายุ"
// @Param   fatherName formData string false "ชื่อบิดา"
// @Param   fatherIdcard formData string false "บัตรประชาชนบิดา"
// @Param   motherName formData string false "ชื่อมารดา"
// @Param   motherIdcard formData string false "บัตรประชาชนมารดา"
// @Param   maritalStatus formData string false "สถานภาพสมรส"
// @Param   deathStatus formData string false "สถานะการเสียชีวิต"
// @Param   dateOfDeath formData string false "วันที่เสียชีวิต"
// @Param   policeRegion formData string false "ภาคตำรวจ"
// @Param   policeProvincial formData string false "ตำรวจภูธรจังหวัด"
// @Param   policeStation formData string false "สถานีตำรวจ"
// @Param   userRecorder formData string false "ผู้บันทึกข้อมูล"
// @Param   userPosition formData string false "ตำแหน่งผู้บันทึก"
// @Param   status formData string false "สถานะ"
// @Param   state formData string false "State"
// @Param   photo formData file false "ไฟล์รูปภาพ"
// @Success 202 {object} gmod.SuccessMessageCreateResponse
// @Failure 400 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.ErrorMessageResponse
// @Router /kwatch/watchlist/{id} [patch]
// @Security BearerAuth
func WatchlistUpdate(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/kwatapi")
	ctx, span := tracer.Start(ctx, "Kwatch.WatchlistUpdate")
	defer span.End()
	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "id path param (watchlist _id หรือ idcard) is required")
	}

	var req kwatmod.WatchlistUpdateRequest

	form, err := c.MultipartForm()
	if err == nil && form.File != nil {
		// ✅ แปลง FormValue → pointer
		req.Type = toPtr(c.FormValue("type"))
		req.PersonalType = toPtr(c.FormValue("personalType"))
		req.CrimesType = toPtr(c.FormValue("crimesType"))
		req.IdCard = toPtr(c.FormValue("idcard"))
		req.Passport = toPtr(c.FormValue("passport"))
		req.TitleName = toPtr(c.FormValue("titlename"))
		req.SubTitleName = toPtr(c.FormValue("subTitlename"))
		req.FirstName = toPtr(c.FormValue("firstname"))
		req.LastName = toPtr(c.FormValue("lastname"))
		req.NickName = toPtr(c.FormValue("nickname"))
		req.Sex = toPtr(c.FormValue("sex"))
		req.Birthday = toPtr(c.FormValue("birthday"))
		req.Age = toPtr(c.FormValue("age"))
		req.FatherName = toPtr(c.FormValue("fatherName"))
		req.FatherIdCard = toPtr(c.FormValue("fatherIdcard"))
		req.MotherName = toPtr(c.FormValue("motherName"))
		req.MotherIdCard = toPtr(c.FormValue("motherIdcard"))
		req.MaritalStatus = toPtr(c.FormValue("maritalStatus"))
		req.DeathStatus = toPtr(c.FormValue("deathStatus"))
		req.DateOfDeath = toPtr(c.FormValue("dateOfDeath"))
		req.PoliceRegion = toPtr(c.FormValue("policeRegion"))
		req.PoliceProvincial = toPtr(c.FormValue("policeProvincial"))
		req.PoliceStation = toPtr(c.FormValue("policeStation"))
		req.UserRecorder = toPtr(c.FormValue("userRecorder"))
		req.UserPosition = toPtr(c.FormValue("userPosition"))
		req.Status = toPtr(c.FormValue("status"))
		req.State = toPtr(c.FormValue("state"))
		req.AlertType = toPtr(c.FormValue("alertType"))
		req.AlertDesc = toPtr(c.FormValue("alertDesc"))
		// ✅ อ่านไฟล์ถ้ามี
		file, err := c.FormFile("photo")
		if err == nil && file != nil {
			f, _ := file.Open()
			defer f.Close()
			bytes, _ := io.ReadAll(f)
			req.PhotoFile = bytes
			req.PhotoFileName = file.Filename
		}
	} else {
		// ✅ ถ้าเป็น JSON → parse ตรงๆ
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
				Code:    "BAD_REQUEST",
				Message: "invalid body",
				Status:  false,
			})
		}
	}

	// ✅ Call service (ปล่อยให้ service แปล id เป็น _id หรือค้นด้วย idcard)
	if err := kwatsvc.WatchlistUpdate(ctx, id, req); err != nil {
		switch {
		case errors.Is(err, kwatsvc.ErrNoFace):
			return httputil.FailBadRequest(c, "BAD_PHOTO_NO_FACE", "No face in this photo")
		case errors.Is(err, kwatsvc.ErrBadPhoto):
			return httputil.FailBadRequest(c, "BAD_PHOTO", "File photo is damaged and cannot be used")
		case errors.Is(err, kwatsvc.ErrInvalidObjectID), errors.Is(err, kwatsvc.ErrWatchlistNotFound):
			return httputil.FailNotFound(c, "NOT_FOUND", "Watchlist not found by _id or idcard")
		default:
			return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to update watchlist")
		}
	}

	return gmod.SendAcceptedWithID(c, "SUCCESS", "Watchlist created successfully", id)
}
