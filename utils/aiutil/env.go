// internal/utils/aiutil/env.go
package aiutil

import "os"

func GetHumanAIApiURL() string {
	return os.Getenv("AI_FACE_ENDPOINT") + "/analyze"
}

func GetVehicleAIApiURL() string {
	return os.Getenv("AI_LPR_ENDPOINT") + "/analyze"
}
