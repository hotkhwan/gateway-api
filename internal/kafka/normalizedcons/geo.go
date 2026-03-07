// internal/kafka/normalizedcons/geo.go
package normalizedcons

import (
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/mmcloughlin/geohash"
)

// computeGeoCell encodes lat/lng into a geohash cell at the configured precision.
// Returns an empty GeoCellInfo (no cell) when lat/lng are zero.
func computeGeoCell(lat, lng float64, cfg GeoConfig) ingestmod.GeoCellInfo {
	info := ingestmod.GeoCellInfo{
		Scheme:    cfg.GeoCellScheme,
		Precision: cfg.GeoCellPrec,
	}
	if lat == 0 && lng == 0 {
		return info
	}
	info.Cell = geohash.EncodeWithPrecision(lat, lng, uint(cfg.GeoCellPrec))
	return info
}

// reverseGeocode resolves lat/lng to an administrative region using the embedded boundary dataset.
// Returns an empty GeoInfo with IdScheme set when lat/lng are zero or outside known boundaries.
func reverseGeocode(lat, lng float64, cfg GeoConfig) ingestmod.GeoInfo {
	info := ingestmod.GeoInfo{IdScheme: cfg.IdScheme}
	if lat == 0 && lng == 0 {
		return info
	}

	region := cfg.BoundaryIndex.Query(lat, lng)
	if region == nil {
		info.CountryCode = cfg.DefaultCountry
		info.AdminLevel = cfg.AdminLevel
		return info
	}

	return ingestmod.GeoInfo{
		CountryCode: region.CountryCode,
		AdminLevel:  region.AdminLevel,
		AdminName:   region.Name,
		AdminCode:   region.Code,
		IdScheme:    cfg.IdScheme,
	}
}

// buildByAdminArea builds the index key used for fast admin-area aggregation queries.
// Result: { "TH-10": {} } — MongoDB wildcard index covers all admin codes.
func buildByAdminArea(geo ingestmod.GeoInfo) ingestmod.ByAdminAreaInfo {
	if geo.AdminCode == "" {
		return ingestmod.ByAdminAreaInfo{}
	}
	return ingestmod.ByAdminAreaInfo{
		geo.AdminCode: map[string]any{},
	}
}
