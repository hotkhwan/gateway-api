// models/subscripmod/package.go
package subscripmod

import "time"

// I18nString — localized string, key = locale code ("th", "en")
type I18nString map[string]string

// PackageFeatures — boolean feature flags included in a plan
type PackageFeatures struct {
	BasicAnalytics       bool `bson:"basicAnalytics"        json:"basicAnalytics"`
	AdvancedAnalytics    bool `bson:"advancedAnalytics"     json:"advancedAnalytics"`
	EmailSupport         bool `bson:"emailSupport"          json:"emailSupport"`
	PriorityEmailSupport bool `bson:"priorityEmailSupport"  json:"priorityEmailSupport"`
	DedicatedSupport24x7 bool `bson:"dedicatedSupport24x7"  json:"dedicatedSupport24x7"`
	CustomIntegrations   bool `bson:"customIntegrations"    json:"customIntegrations"`
	ApiAccess            bool `bson:"apiAccess"             json:"apiAccess"`
	CustomSla            bool `bson:"customSla"             json:"customSla"`
	OnPremise            bool `bson:"onPremise"             json:"onPremise"`
	Sso                  bool `bson:"sso"                   json:"sso"`
	AdvancedSecurity     bool `bson:"advancedSecurity"      json:"advancedSecurity"`
}

// PackagePricing — pricing per billing cycle
type PackagePricing struct {
	Monthly  float64    `bson:"monthly"  json:"monthly"`
	Yearly   float64    `bson:"yearly"   json:"yearly"`
	Currency string     `bson:"currency" json:"currency"`
	Display  I18nString `bson:"display"  json:"display"` // {"th": "ฟรี", "en": "Free"}
}

// PackageBilling — supported billing cycles + pricing
type PackageBilling struct {
	SupportedCycles []string       `bson:"supportedCycles" json:"supportedCycles"`
	Price           PackagePricing `bson:"price"           json:"price"`
}

// PackageUIFeatureItem — one line in the feature list rendered by UI
type PackageUIFeatureItem struct {
	Key   string     `bson:"key"   json:"key"`
	Label I18nString `bson:"label" json:"label"`
}

// PackageUI — hints for frontend rendering (badge, theme, feature list)
type PackageUI struct {
	Badge       I18nString             `bson:"badge"       json:"badge"`
	Highlight   bool                   `bson:"highlight"   json:"highlight"`
	Theme       string                 `bson:"theme"       json:"theme"` // "default" | "primary" | "success"
	FeatureList []PackageUIFeatureItem `bson:"featureList" json:"featureList"`
}

// SubscriptionPackage — collection: subscription_packages
// Master catalog of all plans. Loaded by frontend for pricing page.
// Limits and features are the authoritative source — subscriptionsvc.ResolveEffective
// merges package + subscription.Overrides to get final effective limits.
type SubscriptionPackage struct {
	PackageId   string             `bson:"id"          json:"id"`
	Code        string             `bson:"code"        json:"code"`
	Name        I18nString         `bson:"name"        json:"name"`
	Description I18nString         `bson:"description" json:"description"`
	Status      string             `bson:"status"      json:"status"` // "active" | "inactive"
	SortOrder   int                `bson:"sortOrder"   json:"sortOrder"`
	IsPublic    bool               `bson:"isPublic"    json:"isPublic"`
	Billing     PackageBilling     `bson:"billing"     json:"billing"`
	Limits      SubscriptionLimits `bson:"limits"  json:"limits"`
	Features    PackageFeatures    `bson:"features" json:"features"`
	UI          PackageUI          `bson:"ui"          json:"ui"`
	CreatedAt   time.Time          `bson:"createdAt"   json:"createdAt"`
	UpdatedAt   time.Time          `bson:"updatedAt"   json:"updatedAt"`
}

// EffectiveSubscription — resolved plan + overrides, returned by GET /subscriptions/current
// This is the internal type passed to quota validators.
type EffectiveSubscription struct {
	TenantId     string             `json:"tenantId"`
	PlanId       string             `json:"planId"`
	Status       string             `json:"status"`
	BillingCycle string             `json:"billingCycle"`
	Limits       SubscriptionLimits `json:"limits"`
	Features     PackageFeatures    `json:"features"`
}
