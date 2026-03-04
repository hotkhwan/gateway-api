// models/devsyncmod/commod.go
package devsyncmod

type SyncResult struct {
	Devices         int              `json:"devices"`
	Channels        int              `json:"channels"`
	Inserted        int              `json:"inserted"`
	Updated         int              `json:"updated"`
	PerDeviceCounts map[int64]int    `json:"perDeviceCounts"`
	ChannelsSample  []map[string]any `json:"channelsSample,omitempty"`
}
