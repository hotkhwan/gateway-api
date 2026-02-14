// utils/aiutil/LprPieApple.go
package aiutil

import (
	"bytes"
	"io"
	"klynx/internal/logger"
	"klynx/models/kaimod"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

func LprPieApple(imageBytes []byte, filename, endpoint string) ([]byte, error) {
	log := logger.WithMeta("aiutil", "LprPieApple")
	log.Debug().Int("imgBytes_len", len(imageBytes)).Msg("📦 Image bytes before LPR POST")
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// Manual part header
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="image"; filename="`+filename+`"`)
	h.Set("Content-Type", "image/jpeg") // 💡 ใส่ตรงนี้

	fw, err := w.CreatePart(h)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(imageBytes); err != nil {
		return nil, err
	}
	w.Close()

	req, err := http.NewRequest("POST", endpoint, &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Logs return msg
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Debug().
		Int("resp_len", len(respBody)).
		Int("status", resp.StatusCode).
		Str("endpoint", endpoint).
		Msg("✅ LPR POST response received")

	return respBody, nil // ✅ ใช้ที่อ่านไว้แล้ว

}

func ConvertLpr(src kaimod.SourceLpr) kaimod.LprResult {
	var licensePlate *struct {
		PlateNumber   string  `json:"plateNumber"`
		PlateProvince string  `json:"plateProvince"`
		Confidence    float64 `json:"confidence"`
		Location      struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"location"`
	}

	if src.LicensePlate != nil {
		licensePlate = &struct {
			PlateNumber   string  `json:"plateNumber"`
			PlateProvince string  `json:"plateProvince"`
			Confidence    float64 `json:"confidence"`
			Location      struct {
				X      int `json:"x"`
				Y      int `json:"y"`
				Width  int `json:"width"`
				Height int `json:"height"`
			} `json:"location"`
		}{
			PlateNumber:   src.LicensePlate.PlateNumber,
			PlateProvince: src.LicensePlate.PlateProvince,
			Confidence:    src.LicensePlate.Confidence,
			Location: struct {
				X      int `json:"x"`
				Y      int `json:"y"`
				Width  int `json:"width"`
				Height int `json:"height"`
			}{
				X:      src.LicensePlate.Location.X,
				Y:      src.LicensePlate.Location.Y,
				Width:  src.LicensePlate.Location.Width,
				Height: src.LicensePlate.Location.Height,
			},
		}
	}

	var vehicleInfo *struct {
		Brand struct {
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
		} `json:"brand"`
		Color struct {
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
		} `json:"color"`
		Type struct {
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
		} `json:"type"`
	}

	if src.VehicleInfo != nil {
		vehicleInfo = &struct {
			Brand struct {
				Name       string  `json:"name"`
				Confidence float64 `json:"confidence"`
			} `json:"brand"`
			Color struct {
				Name       string  `json:"name"`
				Confidence float64 `json:"confidence"`
			} `json:"color"`
			Type struct {
				Name       string  `json:"name"`
				Confidence float64 `json:"confidence"`
			} `json:"type"`
		}{
			Brand: src.VehicleInfo.Brand,
			Color: src.VehicleInfo.Color,
			Type:  src.VehicleInfo.Type,
		}
	}

	return kaimod.LprResult{
		LicensePlate: licensePlate,
		VehicleInfo:  vehicleInfo,
	}
}
