// cmd/license.go
//
// Deprecated: this CLI is retained as a thin wrapper over
// internal/services/licensesvc during the migration to the HTTP admin surface
// at /admin/licenses/*. New automation should call the HTTP API instead —
// the CLI will be removed once every caller has switched over.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/internal/services/licensesvc"
	"github.com/hotkhwan/gateway-api/models/subscripmod"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func main() {
	env := flag.String("env", "dev", "Environment: dev / uat / prod")
	action := flag.String("action", "generate", "Action: generate / list / validate / count")
	licenseKey := flag.String("key", "", "License key to validate (for validate action)")
	notes := flag.String("notes", "", "Optional notes for the license (for generate action)")
	flag.Parse()

	envFile := fmt.Sprintf(".env.%s", *env)
	if err := godotenv.Load(envFile); err != nil {
		fmt.Printf("❌ Failed to load env file: %s\n", err)
		os.Exit(1)
	}

	if !strings.EqualFold(os.Getenv("ALLOW_CMD_LIC"), "true") {
		fmt.Println("❌ License CLI is disabled. Set ALLOW_CMD_LIC=true to enable, or use the HTTP admin route /admin/licenses.")
		os.Exit(1)
	}
	if licensesvc.Secret() == "" {
		fmt.Println("❌ LIC_SEC_KEY is required to sign license keys.")
		os.Exit(1)
	}

	logger.Init()
	config.InitMongo()
	log := logger.Boot("license-cli", "main")

	ctx := context.Background()
	repo := subscriprepo.NewLicenseRepo(config.DB)
	svc := licensesvc.New(repo)

	log.Info().Msg("🔑 License Key Management Tool (HTTP admin surface preferred — this CLI is deprecated)")

	switch *action {
	case "generate":
		license, err := svc.Issue(ctx, licensesvc.IssueOptions{
			Notes: strPtrOrNil(*notes),
		})
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to create license")
		}
		printGenerated(license)

	case "list":
		licenses, err := svc.List(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to list licenses")
		}
		printList(licenses)

	case "validate":
		if *licenseKey == "" {
			fmt.Println("❌ -key is required for validate")
			os.Exit(1)
		}
		license, err := svc.Validate(ctx, *licenseKey)
		if err != nil {
			printValidationError(err)
			os.Exit(1)
		}
		fmt.Println("✅ License key is valid and available for activation!")
		fmt.Printf("   Key:     %s\n", license.Key)
		fmt.Printf("   Status:  %s\n", license.Status)
		fmt.Printf("   Created: %s\n", license.CreatedAt.Format(time.RFC3339))

	case "count":
		for _, status := range []subscripmod.LicenseStatus{
			subscripmod.LicenseStatusAvailable,
			subscripmod.LicenseStatusActivated,
			subscripmod.LicenseStatusRevoked,
		} {
			count, err := repo.CountByStatus(ctx, status)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to count %s licenses", status)
				continue
			}
			fmt.Printf("   %-12s: %d\n", status, count)
		}

	default:
		fmt.Println("❌ Invalid action. Valid: generate, list, validate, count")
		os.Exit(1)
	}
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func printGenerated(lic *subscripmod.LicenseKey) {
	fmt.Println("\n✅ License Key Generated Successfully!")
	fmt.Println("=====================================")
	fmt.Printf("ID:           %s\n", lic.ID.Hex())
	fmt.Printf("License Key:  %s\n", lic.Key)
	fmt.Printf("Plan:         %s\n", lic.PlanId)
	fmt.Printf("Status:       %s\n", lic.Status)
	fmt.Printf("Created At:   %s\n", lic.CreatedAt.Format(time.RFC3339))
	if lic.Notes != nil {
		fmt.Printf("Notes:        %s\n", *lic.Notes)
	}
}

func printList(licenses []*subscripmod.LicenseKey) {
	fmt.Printf("\n📋 License Keys (Total: %d)\n", len(licenses))
	fmt.Println("=====================================")
	for i, lic := range licenses {
		fmt.Printf("\n[%d] ID: %s\n", i+1, lic.ID.Hex())
		fmt.Printf("    Key:     %s\n", lic.Key)
		fmt.Printf("    Status:  %s\n", lic.Status)
		fmt.Printf("    Plan:    %s\n", lic.PlanId)
		fmt.Printf("    Created: %s\n", lic.CreatedAt.Format(time.RFC3339))
		if lic.TenantId != nil {
			fmt.Printf("    Tenant:  %s\n", *lic.TenantId)
			if lic.ActivatedAt != nil {
				fmt.Printf("    Activated: %s\n", lic.ActivatedAt.Format(time.RFC3339))
			}
		}
		if lic.Notes != nil {
			fmt.Printf("    Notes:   %s\n", *lic.Notes)
		}
	}
}

func printValidationError(err error) {
	switch {
	case errors.Is(err, licensesvc.ErrInvalidKey):
		fmt.Println("❌ License key format is invalid (expected " + licensesvc.KeyFormat + ")")
	case errors.Is(err, subscriprepo.ErrLicenseNotFound):
		fmt.Println("❌ License key not found in database")
	case errors.Is(err, subscriprepo.ErrLicenseAlreadyActivated):
		fmt.Println("⚠️  License key has already been activated")
	case errors.Is(err, subscriprepo.ErrLicenseRevoked):
		fmt.Println("⚠️  License key has been revoked")
	default:
		fmt.Printf("❌ Validation error: %s\n", err.Error())
	}
}

// keep primitive import referenced so goimports doesn't drop it when the
// CLI eventually grows subcommands that pass ObjectID.
var _ = primitive.NilObjectID
