// internal/services/kwatsvc/wiring.go
package kwatsvc

import (
	"os"

	"github.com/hotkhwan/gateway-api/internal/gateways/iboc/watchlist/ibface"
	"github.com/hotkhwan/gateway-api/internal/gateways/watchman/watchgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
)

type Gateways struct {
	IBOCProd *ibface.Client
	IBOCDev  *ibface.Client
	Watchman *watchgw.Client
}

func NewGatewaysFromEnv() Gateways {
	g := Gateways{
		IBOCProd: ibface.NewFromEnv("IBOC_6"),
		IBOCDev:  ibface.NewFromEnv("IBOC_DEV"),
		Watchman: watchgw.NewFromEnv("WATCHMAN"),
	}
	if g.IBOCProd != nil && (g.IBOCProd.BaseURL == "" || g.IBOCProd.Token == "") {
		g.IBOCProd = nil
	}
	if g.IBOCDev != nil && (g.IBOCDev.BaseURL == "" || g.IBOCDev.Token == "") {
		g.IBOCDev = nil
	}
	if g.Watchman != nil && !g.Watchman.Configured() {
		g.Watchman = nil
	}
	return g
}

func (g Gateways) LogWiring() {
	log := logger.WithMeta("kwatsvc", "wiring")
	log.Info().
		Str("IBOC_6_API", os.Getenv("IBOC_6_API")).
		Str("IBOC_DEV_API", os.Getenv("IBOC_DEV_API")).
		Str("WATCHMAN_API", os.Getenv("WATCHMAN_API")).
		Bool("enabled.iboc", g.IBOCProd != nil).
		Bool("enabled.ibocdev", g.IBOCDev != nil).
		Bool("enabled.watchman", g.Watchman != nil).
		Msg("🔧 External gateways wiring (non-secret)")
}

// ช่วยคืนรายชื่อ namespace ที่เปิดใช้อยู่จริง
func (g Gateways) ConfiguredNamespaces() []string {
	ns := make([]string, 0, 3)
	if g.IBOCProd != nil {
		ns = append(ns, "iboc")
	}
	if g.IBOCDev != nil {
		ns = append(ns, "ibocdev")
	}
	if g.Watchman != nil {
		ns = append(ns, "watchman")
	}
	return ns
}
