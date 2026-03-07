// internal/geoboundary/geoboundary.go
package geoboundary

import "math"

// AdminRegion represents an administrative region returned by a boundary query.
type AdminRegion struct {
	CountryCode string // ISO 3166-1 alpha-2, e.g. "TH"
	AdminLevel  int    // 1 = province, 2 = district
	Name        string // e.g. "Bangkok"
	Code        string // ISO 3166-2, e.g. "TH-10"
}

// BoundaryQuerier returns the admin region for a given lat/lng.
type BoundaryQuerier interface {
	Query(lat, lng float64) *AdminRegion
}

// LoadEmbedded returns a BoundaryQuerier backed by the embedded static TH dataset.
func LoadEmbedded() BoundaryQuerier {
	return &thBoundaryIndex{}
}

// thBoundaryIndex implements BoundaryQuerier using centroid-based lookup.
type thBoundaryIndex struct{}

type provinceEntry struct {
	code      string // ISO 3166-2
	name      string
	centerLat float64
	centerLng float64
	minLat    float64
	maxLat    float64
	minLng    float64
	maxLng    float64
}

// Query finds the Thai province whose bounding box contains the point.
// Falls back to closest centroid within 200 km if multiple boxes match.
// Returns nil if the point is outside Thailand.
func (b *thBoundaryIndex) Query(lat, lng float64) *AdminRegion {
	if lat == 0 && lng == 0 {
		return nil
	}
	// Rough Thailand bbox check
	if lat < 5.5 || lat > 20.5 || lng < 97.3 || lng > 105.7 {
		return nil
	}

	var best *provinceEntry
	bestDist := math.MaxFloat64

	for i := range thProvinces {
		p := &thProvinces[i]
		if lat >= p.minLat && lat <= p.maxLat && lng >= p.minLng && lng <= p.maxLng {
			d := dist(lat, lng, p.centerLat, p.centerLng)
			if d < bestDist {
				bestDist = d
				best = p
			}
		}
	}

	if best == nil {
		// fallback: closest centroid within 200 km
		for i := range thProvinces {
			p := &thProvinces[i]
			d := dist(lat, lng, p.centerLat, p.centerLng)
			if d < bestDist {
				bestDist = d
				best = p
			}
		}
		if bestDist > 200 {
			return nil
		}
	}

	return &AdminRegion{
		CountryCode: "TH",
		AdminLevel:  1,
		Name:        best.name,
		Code:        best.code,
	}
}

// dist returns approximate distance in km between two points (equirectangular).
func dist(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	midLat := (lat1 + lat2) * math.Pi / 180
	x := dLng * math.Cos(midLat)
	return math.Sqrt(x*x+dLat*dLat) * R
}
