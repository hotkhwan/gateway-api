// internal/services/authsvc/signin.go
package authsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/authmod"
	"github.com/hotkhwan/gateway-api/utils/authutil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func Authenticate(ctx context.Context, req authmod.SigninRequest) (authmod.SigninResponse, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/authsvc", "authenticate.singin", "authsvc", "Authenticate")
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Debug().Str("username", req.Username).Str("hwId", req.HwID).Msg("🔐 Starting authentication")

	// ---- Keycloak ----
	form := url.Values{}
	form.Add("client_id", os.Getenv("KEYCLOAK_CLIENT_ID"))
	form.Add("client_secret", os.Getenv("KEYCLOAK_CLIENT_SECRET"))
	form.Add("grant_type", "password")
	form.Add("scope", "openid")
	form.Add("username", req.Username)
	form.Add("password", req.Password)

	keycloakURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		os.Getenv("KEYCLOAK_URL"), os.Getenv("KEYCLOAK_REALM"))

	resp, err := http.PostForm(keycloakURL, form)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to post to Keycloak")
		return authmod.SigninResponse{}, errors.New("unauthorized")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Warn().Int("status", resp.StatusCode).Msg("⚠️ Keycloak returned non-200")
		return authmod.SigninResponse{}, errors.New("unauthorized")
	}

	var tokenData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tokenData); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode token response")
		return authmod.SigninResponse{}, err
	}

	claims, err := authutil.DecodeJWT(tokenData["access_token"].(string))
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode access token")
		return authmod.SigninResponse{}, err
	}

	role, _ := claims["role"].(string)
	if role == "" {
		role = "user"
	}

	response := authmod.SigninResponse{
		Sub:          fmt.Sprintf("%v", claims["sub"]),
		Username:     fmt.Sprintf("%v", claims["preferred_username"]),
		Name:         fmt.Sprintf("%v", claims["name"]),
		Email:        fmt.Sprintf("%v", claims["email"]),
		Role:         role,
		Locale:       fmt.Sprintf("%v", claims["locale"]),
		Plant:        claims["plant"],
		AccessToken:  tokenData["access_token"].(string),
		RefreshToken: tokenData["refresh_token"].(string),
	}

	// ---- HwID / Station ----
	if req.HwID != "" {
		if err := fillStationInfo(ctx, req.HwID, &response); err != nil {
			return authmod.SigninResponse{}, err
		}
	}

	log.Debug().Str("username", req.Username).Msg("✅ Authentication successful")
	return response, nil
}

func fillStationInfo(_ context.Context, hwID string, resp *authmod.SigninResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	stations := db.Collection("stations")

	var result map[string]interface{}
	err := stations.FindOne(ctx, bson.M{
		"hwId":   hwID,
		"state":  "create",
		"verify": true,
	}).Decode(&result)

	if err == nil {
		resp.StationID = fmt.Sprintf("%v", result["_id"])
		resp.StationName = fmt.Sprintf("%v", result["name"])
		resp.StationHwId = fmt.Sprintf("%v", result["hwId"])
		resp.StationType = fmt.Sprintf("%v", result["stationType"])
		resp.StationAddress = fmt.Sprintf("%v", result["ipAddress"])
		resp.StationCamera = fmt.Sprintf("%v", result["cameraURL"])
		return nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return errors.New("invalid username or password")
	}
	return err
}
