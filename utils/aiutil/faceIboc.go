// utils/aiutil/LprPieApple.go
package aiutil

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

func FaceIboc(imageBytes []byte, filename, endpoint string) ([]byte, error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, err := w.CreateFormFile("image", filename)
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

	return io.ReadAll(resp.Body)
}
