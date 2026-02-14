// utils/aiutil/ibocmapper.go
package aiutil

import (
	"fmt"
	"strings"

	"klynx/models/kwatmod"
	"klynx/utils"
)

type IBOCBuildCfg struct {
	Email            string
	Enabled          bool
	StaticTags       []string
	StaticMetadata   map[string]interface{}
	TagsFrom         []string          // ex: []{"PersonalType","Age"}
	MetadataFrom     map[string]string // ex: map["NickName"]="NickName"
	CoalesceIdentity bool              // true = ใช้ IdCard ก่อน ถ้าไม่มีค่อยใช้ Passport
}

// MapWatchlistEventToIBOC แปลง WatchlistEvent + Config → CreateWatchlistIBOCRequest
func MapWatchlistEventToIBOC(evt kwatmod.WatchlistEvent, cfg IBOCBuildCfg) (kwatmod.CreateWatchlistIBOCRequest, error) {
	out := kwatmod.CreateWatchlistIBOCRequest{
		FirstName: evt.FirstName,
		LastName:  evt.LastName,
		Email:     cfg.Email,
		Enabled:   cfg.Enabled,
		Tags:      nil,
		Metadata:  map[string]interface{}{},
	}

	// identity
	if cfg.CoalesceIdentity {
		out.IdentityDocId = firstNonEmpty(evt.IdCard, evt.Passport)
	} else {
		out.IdentityDocId = evt.IdCard
	}

	// tags
	var tags []string
	for _, f := range cfg.TagsFrom {
		if v := eventFieldString(evt, f); v != "" {
			tags = append(tags, v)
		}
	}
	tags = append(tags, cfg.StaticTags...)
	out.Tags = uniqNonEmpty(tags)

	// metadata
	for k, field := range cfg.MetadataFrom {
		if v := eventFieldString(evt, field); v != "" {
			out.Metadata[k] = v
		}
	}
	for k, v := range cfg.StaticMetadata {
		out.Metadata[k] = v
	}

	if strings.TrimSpace(out.FirstName) == "" {
		return out, fmt.Errorf("firstName is required")
	}
	return out, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func eventFieldString(evt kwatmod.WatchlistEvent, name string) string {
	switch name {
	case "ID":
		return evt.ID
	case "Event":
		return evt.Event
	case "Time":
		return evt.Time
	case "PhotoURL":
		return evt.PhotoURL
	case "PhotoFileName":
		return evt.PhotoFileName
	case "PhotoContentType":
		return evt.PhotoContentType
	case "Type":
		return utils.Itoa(evt.Type)
	case "PersonalType":
		return utils.Itoa(evt.PersonalType)
	case "CrimesType":
		return utils.Itoa(evt.CrimesType)
	case "IdCard":
		return evt.IdCard
	case "Passport":
		return evt.Passport
	case "TitleName":
		return evt.TitleName
	case "SubTitleName":
		return evt.SubTitleName
	case "FirstName":
		return evt.FirstName
	case "LastName":
		return evt.LastName
	case "NickName":
		return evt.NickName
	case "Sex":
		return evt.Sex
	case "Birthday":
		return evt.Birthday
	case "Age":
		return utils.Itoa(evt.Age)
	case "FatherName":
		return evt.FatherName
	case "FatherIdCard":
		return evt.FatherIdCard
	case "MotherName":
		return evt.MotherName
	case "MotherIdCard":
		return evt.MotherIdCard
	case "MaritalStatus":
		return evt.MaritalStatus
	case "DeathStatus":
		return utils.Itoa(evt.DeathStatus)
	case "DateOfDeath":
		return evt.DateOfDeath
	case "PoliceRegion":
		return utils.Itoa(evt.PoliceRegion)
	case "PoliceProvincial":
		return utils.Itoa(evt.PoliceProvincial)
	case "PoliceStation":
		return utils.Itoa(evt.PoliceStation)
	case "UserRecorder":
		return evt.UserRecorder
	case "UserPosition":
		return evt.UserPosition
	case "State":
		return evt.State
	default:
		return ""
	}
}

func uniqNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
