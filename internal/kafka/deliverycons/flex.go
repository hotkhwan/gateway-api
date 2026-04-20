// internal/kafka/deliverycons/flex.go
package deliverycons

import (
	"context"
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// googleMapsURL builds a Google Maps deep-link from lat/lng. LINE's uri action
// opens this URL in the native map app when the user taps the address row
// (or hero image) on a Flex bubble. Using the geo URI scheme would be more
// universal but Google Maps works reliably on both iOS and Android.
func googleMapsURL(lat, lng float64) string {
	return fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%f,%f", lat, lng)
}

// tagColorPalette maps the six tag color presets (matching the LINE OA card
// settings UI) to hex values used in Flex bubble styling.
var tagColorPalette = map[string]struct {
	Bg   string
	Text string
}{
	"gray":   {Bg: "#6B7280", Text: "#FFFFFF"},
	"white":  {Bg: "#FFFFFF", Text: "#111827"},
	"red":    {Bg: "#EF4444", Text: "#FFFFFF"},
	"orange": {Bg: "#F97316", Text: "#FFFFFF"},
	"green":  {Bg: "#10B981", Text: "#FFFFFF"},
	"blue":   {Bg: "#3B82F6", Text: "#FFFFFF"},
}

// additionalInfoIcon maps the extras.additionalInfoType selector to a unicode
// glyph rendered inline (LINE Flex accepts icons via URL but a text glyph is
// simpler and consistent with the OA Card prototype).
var additionalInfoIcon = map[string]string{
	"hours": "🕐",
	"clock": "🕐",
	"info":  "ℹ️",
	"note":  "📝",
	"phone": "📞",
	"tag":   "🏷️",
}

// flexOpts captures the resolved configuration for one Flex card render.
type flexOpts struct {
	Header              string
	HeroURL             string
	TagText             string
	TagColor            string
	Address             string
	AdditionalInfoType  string
	AdditionalInfoText  string
	Action1Label        string
	Action1URL          string
	Action2Label        string
	Action2URL          string
	AddressEnabled      bool
	TagEnabled          bool
	AdditionalEnabled   bool
	Action1Enabled      bool
	Action2Enabled      bool
}

// buildFlexCard assembles a LINE Flex Bubble JSON (as a generic map) that
// matches the OA Card layout: hero image, tag, header, address row,
// additional-info row, and up to two footer action buttons.
//
// Returns nil when there is nothing meaningful to render (no header, no body,
// no image) so the caller can fall back to plain text.
func buildFlexCard(ctx context.Context, event *ingestmod.NormalizedEvent, tmpl *ingestmod.MappingTemplate, mt *ingestmod.MessageTemplate, renderedTitle, renderedBody string) map[string]any {
	opts := resolveFlexOpts(ctx, event, mt, renderedTitle, renderedBody)

	body := []map[string]any{}
	if opts.TagEnabled && opts.TagText != "" {
		body = append(body, flexTagBox(opts.TagText, opts.TagColor))
	}
	if opts.Header != "" {
		body = append(body, map[string]any{
			"type":   "text",
			"text":   opts.Header,
			"weight": "bold",
			"size":   "lg",
			"wrap":   true,
		})
	}
	if opts.AddressEnabled && opts.Address != "" {
		row := flexIconRow("📍", opts.Address)
		// Attach Google Maps URI action when lat/lng are available so tapping
		// the address row on LINE opens the device location in the native map
		// app (matches the "Location info" feature in LINE OA card messages).
		if event.Location.Lat != 0 || event.Location.Lng != 0 {
			row["action"] = map[string]any{
				"type": "uri",
				"uri":  googleMapsURL(event.Location.Lat, event.Location.Lng),
			}
		}
		body = append(body, row)
	}
	if opts.AdditionalEnabled && opts.AdditionalInfoText != "" {
		icon := additionalInfoIcon[strings.ToLower(strings.TrimSpace(opts.AdditionalInfoType))]
		if icon == "" {
			icon = "🕐"
		}
		body = append(body, flexIconRow(icon, opts.AdditionalInfoText))
	}
	if len(body) == 0 && opts.HeroURL == "" {
		return nil
	}

	bubble := map[string]any{
		"type": "bubble",
		"size": "mega",
	}
	if opts.HeroURL != "" {
		hero := flexHero(opts.HeroURL)
		// When we have a location, tapping the hero image opens Google Maps
		// — same UX as the LINE OA Card "Location info" attachment.
		if event.Location.Lat != 0 || event.Location.Lng != 0 {
			hero["action"] = map[string]any{
				"type": "uri",
				"uri":  googleMapsURL(event.Location.Lat, event.Location.Lng),
			}
		}
		bubble["hero"] = hero
	}
	if len(body) > 0 {
		bubble["body"] = map[string]any{
			"type":     "box",
			"layout":   "vertical",
			"spacing":  "md",
			"contents": body,
		}
	}
	footer := flexFooter(opts)
	if footer != nil {
		bubble["footer"] = footer
	}
	return bubble
}

// resolveFlexOpts reads the message template's extras map and template fields,
// presigns the first binaryRef, and materialises a flexOpts struct for the
// bubble builder.
func resolveFlexOpts(ctx context.Context, event *ingestmod.NormalizedEvent, mt *ingestmod.MessageTemplate, renderedTitle, renderedBody string) flexOpts {
	extras := map[string]string{}
	if mt != nil && mt.Extras != nil {
		extras = mt.Extras
	}
	data := renderContext(event)

	render := func(raw string) string {
		if raw == "" {
			return ""
		}
		if out, err := renderText(raw, data); err == nil {
			return out
		}
		return raw
	}

	opts := flexOpts{
		Header:             renderedTitle,
		Address:            renderedBody,
		TagText:            render(extras["tagText"]),
		TagColor:           strings.ToLower(strings.TrimSpace(extras["tagColor"])),
		AdditionalInfoType: extras["additionalInfoType"],
		AdditionalInfoText: render(extras["additionalInfoText"]),
		Action1Label:       render(extras["action1Label"]),
		Action1URL:         render(extras["action1Url"]),
		Action2Label:       render(extras["action2Label"]),
		Action2URL:         render(extras["action2Url"]),
		TagEnabled:         extras["tagEnabled"] == "true",
		AddressEnabled:     extras["addressEnabled"] != "false",
		AdditionalEnabled:  extras["additionalInfoEnabled"] == "true",
		Action1Enabled:     extras["action1Enabled"] == "true",
		Action2Enabled:     extras["action2Enabled"] == "true",
	}

	// Default tag text falls back to eventCategory for convenience when the
	// template author enabled the tag without specifying tagText.
	if opts.TagEnabled && opts.TagText == "" {
		opts.TagText = event.EventCategory
	}

	if len(event.BinaryRefs) > 0 {
		ref := event.BinaryRefs[0]
		if strings.HasPrefix(ref.Kind, "image") || strings.HasPrefix(ref.ContentType, "image/") || ref.Kind == "image" {
			if url, err := config.PresignS3GetURL(ctx, ref.Bucket, ref.ObjectId); err == nil {
				opts.HeroURL = url
			}
		}
	}

	return opts
}

func flexHero(url string) map[string]any {
	return map[string]any{
		"type":        "image",
		"url":         url,
		"size":        "full",
		"aspectRatio": "16:9",
		"aspectMode":  "cover",
	}
}

func flexTagBox(text, color string) map[string]any {
	palette, ok := tagColorPalette[color]
	if !ok {
		palette = tagColorPalette["gray"]
	}
	return map[string]any{
		"type":            "box",
		"layout":          "horizontal",
		"backgroundColor": palette.Bg,
		"cornerRadius":    "md",
		"paddingAll":      "sm",
		"contents": []map[string]any{
			{
				"type":  "text",
				"text":  text,
				"size":  "xs",
				"color": palette.Text,
				"align": "center",
			},
		},
	}
}

func flexIconRow(icon, text string) map[string]any {
	return map[string]any{
		"type":    "box",
		"layout":  "baseline",
		"spacing": "sm",
		"contents": []map[string]any{
			{"type": "text", "text": icon, "size": "sm", "flex": 0},
			{"type": "text", "text": text, "size": "sm", "color": "#555555", "wrap": true, "flex": 5},
		},
	}
}

func flexFooter(opts flexOpts) map[string]any {
	buttons := []map[string]any{}
	if opts.Action1Enabled && opts.Action1Label != "" && opts.Action1URL != "" {
		buttons = append(buttons, flexURLButton(opts.Action1Label, opts.Action1URL, "primary"))
	}
	if opts.Action2Enabled && opts.Action2Label != "" && opts.Action2URL != "" {
		buttons = append(buttons, flexURLButton(opts.Action2Label, opts.Action2URL, "secondary"))
	}
	if len(buttons) == 0 {
		return nil
	}
	return map[string]any{
		"type":     "box",
		"layout":   "vertical",
		"spacing":  "sm",
		"contents": buttons,
	}
}

func flexURLButton(label, url, style string) map[string]any {
	return map[string]any{
		"type":   "button",
		"style":  style,
		"height": "sm",
		"action": map[string]any{
			"type":  "uri",
			"label": label,
			"uri":   url,
		},
	}
}
