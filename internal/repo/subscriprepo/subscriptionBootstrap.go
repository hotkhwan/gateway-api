// internal/repo/subscriprepo/subscriptionBootstrap.go
package subscriprepo

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/subscripmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		if err := NewSubscriptionRepo(config.DB).EnsureIndexes(ctx); err != nil {
			return err
		}
		pkgRepo := NewPackageRepo()
		if err := pkgRepo.EnsureIndexes(ctx); err != nil {
			return err
		}
		return seedPackages(ctx, pkgRepo)
	})
}

// EnsureIndexes ensures required indexes for subscriptions collection
func (r *SubscriptionRepo) EnsureIndexes(ctx context.Context) error {
	// Unique index on tenantId (one tenant = one active subscription)
	if err := stomongo.EnsureUniqueIndex(
		ctx,
		"subscriptions",
		bson.D{
			{Key: "tenantId", Value: 1},
		},
		"uq_tenantId",
	); err != nil {
		return err
	}

	// Index on status + updatedAt for ops/debug
	statusIdx := mongo.IndexModel{
		Keys: bson.D{
			{Key: "status", Value: 1},
			{Key: "updatedAt", Value: -1},
		},
		Options: options.Index().SetName("idx_status_updatedAt"),
	}
	if err := stomongo.CreateIndexes(ctx, "subscriptions", []mongo.IndexModel{statusIdx}); err != nil {
		return err
	}

	// Index on planId for querying by plan
	planIdx := mongo.IndexModel{
		Keys: bson.D{
			{Key: "planId", Value: 1},
		},
		Options: options.Index().SetName("idx_planId"),
	}
	if err := stomongo.CreateIndexes(ctx, "subscriptions", []mongo.IndexModel{planIdx}); err != nil {
		return err
	}

	return nil
}

// seedPackages upserts freemium/pro/enterprise into subscription_packages.
// Idempotent — safe to run on every startup.
func seedPackages(ctx context.Context, repo *PackageRepo) error {
	now := time.Now().UTC()

	packages := []subscripmod.SubscriptionPackage{
		{
			PackageId:   "freemium",
			Code:        "freemium",
			Name:        subscripmod.I18nString{"th": "Freemium", "en": "Freemium"},
			Description: subscripmod.I18nString{"th": "เริ่มต้นใช้งานด้วยฟีเจอร์พื้นฐาน", "en": "Get started with basic features"},
			Status:      "active",
			SortOrder:   100,
			IsPublic:    true,
			Billing: subscripmod.PackageBilling{
				SupportedCycles: []string{"monthly", "yearly"},
				Price: subscripmod.PackagePricing{
					Monthly:  0,
					Yearly:   0,
					Currency: "USD",
					Display:  subscripmod.I18nString{"th": "ฟรี", "en": "Free"},
				},
			},
			Limits: subscripmod.SubscriptionLimits{
				OrgCacheTtlSec:            30,
				MaxPayloadBytes:           1 * 1024 * 1024,
				PerOrgPerSec:              10,
				PerOrgBurst:               20,
				PerIpPerMin:               300,
				StorageQuotaBytes:         10 * 1024 * 1024 * 1024,
				MaxOrganizationsPerTenant: 1,
				EventsPerMonth:            1000,
				TeamMembers:               5,
				WebhooksPerOrg:            1,
				LineTargetsPerOrg:         1,
				DiscordTargetsPerOrg:      1,
				TelegramTargetsPerOrg:     1,
				MessageChannelsPerOrg:     1,
			},
			Features: subscripmod.PackageFeatures{
				BasicAnalytics: true,
				EmailSupport:   true,
			},
			UI: subscripmod.PackageUI{
				Badge:     subscripmod.I18nString{"th": "", "en": ""},
				Highlight: false,
				Theme:     "default",
				FeatureList: []subscripmod.PackageUIFeatureItem{
					{Key: "organizations", Label: subscripmod.I18nString{"th": "1 Organization", "en": "1 Organization"}},
					{Key: "teamMembers", Label: subscripmod.I18nString{"th": "5 Team Members", "en": "5 Team Members"}},
					{Key: "eventsPerMonth", Label: subscripmod.I18nString{"th": "1,000 Events/เดือน", "en": "1,000 Events/Month"}},
					{Key: "webhooksPerOrg", Label: subscripmod.I18nString{"th": "Webhook 1 ต่อ org", "en": "1 Webhook per org"}},
					{Key: "messageChannels", Label: subscripmod.I18nString{"th": "1 Message Channel ต่อ org", "en": "1 Message Channel per org"}},
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			PackageId:   "pro",
			Code:        "pro",
			Name:        subscripmod.I18nString{"th": "Pro", "en": "Pro"},
			Description: subscripmod.I18nString{"th": "สำหรับทีมที่กำลังเติบโต", "en": "For growing teams"},
			Status:      "active",
			SortOrder:   200,
			IsPublic:    true,
			Billing: subscripmod.PackageBilling{
				SupportedCycles: []string{"monthly", "yearly"},
				Price: subscripmod.PackagePricing{
					Monthly:  49,
					Yearly:   490,
					Currency: "USD",
					Display:  subscripmod.I18nString{"th": "$49/เดือน", "en": "$49/month"},
				},
			},
			Limits: subscripmod.SubscriptionLimits{
				OrgCacheTtlSec:            90,
				MaxPayloadBytes:           40 * 1024 * 1024,
				PerOrgPerSec:              100,
				PerOrgBurst:               200,
				PerIpPerMin:               3000,
				StorageQuotaBytes:         100 * 1024 * 1024 * 1024,
				MaxOrganizationsPerTenant: 5,
				EventsPerMonth:            100000,
				TeamMembers:               25,
				WebhooksPerOrg:            2,
				LineTargetsPerOrg:         1,
				DiscordTargetsPerOrg:      1,
				TelegramTargetsPerOrg:     1,
				MessageChannelsPerOrg:     3,
			},
			Features: subscripmod.PackageFeatures{
				BasicAnalytics:       true,
				AdvancedAnalytics:    true,
				EmailSupport:         true,
				PriorityEmailSupport: true,
				CustomIntegrations:   true,
				ApiAccess:            true,
			},
			UI: subscripmod.PackageUI{
				Badge:     subscripmod.I18nString{"th": "แนะนำ", "en": "Recommended"},
				Highlight: true,
				Theme:     "primary",
				FeatureList: []subscripmod.PackageUIFeatureItem{
					{Key: "organizations", Label: subscripmod.I18nString{"th": "5 Organizations", "en": "5 Organizations"}},
					{Key: "teamMembers", Label: subscripmod.I18nString{"th": "25 Team Members", "en": "25 Team Members"}},
					{Key: "eventsPerMonth", Label: subscripmod.I18nString{"th": "100,000 Events/เดือน", "en": "100,000 Events/Month"}},
					{Key: "webhooksPerOrg", Label: subscripmod.I18nString{"th": "Webhook 2 ต่อ org", "en": "2 Webhooks per org"}},
					{Key: "messageChannels", Label: subscripmod.I18nString{"th": "3 Message Channels ต่อ org", "en": "3 Message Channels per org"}},
					{Key: "advancedAnalytics", Label: subscripmod.I18nString{"th": "Advanced Analytics", "en": "Advanced Analytics"}},
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			PackageId:   "enterprise",
			Code:        "enterprise",
			Name:        subscripmod.I18nString{"th": "Enterprise", "en": "Enterprise"},
			Description: subscripmod.I18nString{"th": "สำหรับองค์กรขนาดใหญ่", "en": "For large-scale deployments"},
			Status:      "active",
			SortOrder:   300,
			IsPublic:    false,
			Billing: subscripmod.PackageBilling{
				SupportedCycles: []string{"monthly", "yearly"},
				Price: subscripmod.PackagePricing{
					Monthly:  0,
					Yearly:   0,
					Currency: "USD",
					Display:  subscripmod.I18nString{"th": "ติดต่อเรา", "en": "Contact Us"},
				},
			},
			Limits: subscripmod.SubscriptionLimits{
				OrgCacheTtlSec:            120,
				MaxPayloadBytes:           100 * 1024 * 1024,
				PerOrgPerSec:              1000,
				PerOrgBurst:               2000,
				PerIpPerMin:               10000,
				StorageQuotaBytes:         1024 * 1024 * 1024 * 1024,
				MaxOrganizationsPerTenant: -1,
				EventsPerMonth:            -1,
				TeamMembers:               -1,
				WebhooksPerOrg:            3,
				LineTargetsPerOrg:         1,
				DiscordTargetsPerOrg:      1,
				TelegramTargetsPerOrg:     1,
				MessageChannelsPerOrg:     3,
			},
			Features: subscripmod.PackageFeatures{
				BasicAnalytics:       true,
				AdvancedAnalytics:    true,
				EmailSupport:         true,
				PriorityEmailSupport: true,
				DedicatedSupport24x7: true,
				CustomIntegrations:   true,
				ApiAccess:            true,
				CustomSla:            true,
				OnPremise:            true,
				Sso:                  true,
				AdvancedSecurity:     true,
			},
			UI: subscripmod.PackageUI{
				Badge:     subscripmod.I18nString{"th": "แผนปัจจุบัน", "en": "Current plan"},
				Highlight: true,
				Theme:     "success",
				FeatureList: []subscripmod.PackageUIFeatureItem{
					{Key: "organizations", Label: subscripmod.I18nString{"th": "Unlimited Organizations", "en": "Unlimited Organizations"}},
					{Key: "teamMembers", Label: subscripmod.I18nString{"th": "Unlimited Team Members", "en": "Unlimited Team Members"}},
					{Key: "eventsPerMonth", Label: subscripmod.I18nString{"th": "Unlimited Events", "en": "Unlimited Events"}},
					{Key: "webhooksPerOrg", Label: subscripmod.I18nString{"th": "Webhook 3 ต่อ org", "en": "3 Webhooks per org"}},
					{Key: "messageChannels", Label: subscripmod.I18nString{"th": "3 Message Channels ต่อ org", "en": "3 Message Channels per org"}},
					{Key: "dedicatedSupport", Label: subscripmod.I18nString{"th": "24/7 Dedicated Support", "en": "24/7 Dedicated Support"}},
					{Key: "onPremise", Label: subscripmod.I18nString{"th": "On-premise Option", "en": "On-premise Option"}},
					{Key: "sso", Label: subscripmod.I18nString{"th": "SSO & Advanced Security", "en": "SSO & Advanced Security"}},
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	for _, pkg := range packages {
		p := pkg
		if err := repo.Upsert(ctx, &p); err != nil {
			return err
		}
	}
	return nil
}
