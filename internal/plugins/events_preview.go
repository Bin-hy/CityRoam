package plugins

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
)

const previewLimit = 3

type EventPreview struct {
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle,omitempty"`
	PriceYuan int    `json:"price_yuan,omitempty"`
}

type EventPreviewer interface {
	QuerySummary(input json.RawMessage) string
	ResultPreview(output json.RawMessage) []EventPreview
}

func queryFor(tool agent.ResearchTool, input json.RawMessage) string {
	if tool == nil || len(input) == 0 {
		return ""
	}
	previewer, ok := tool.(EventPreviewer)
	if !ok {
		return ""
	}
	return strings.TrimSpace(previewer.QuerySummary(input))
}

func previewFor(tool agent.ResearchTool, output json.RawMessage) []EventPreview {
	if tool == nil || len(output) == 0 {
		return nil
	}
	previewer, ok := tool.(EventPreviewer)
	if !ok {
		return nil
	}
	items := previewer.ResultPreview(output)
	if len(items) > previewLimit {
		items = items[:previewLimit]
	}
	out := make([]EventPreview, 0, len(items))
	for _, item := range items {
		item.Title = clipRunes(strings.TrimSpace(item.Title), 80)
		item.Subtitle = clipRunes(strings.TrimSpace(item.Subtitle), 80)
		if item.PriceYuan < 0 {
			item.PriceYuan = 0
		}
		if item.Title == "" && item.Subtitle == "" && item.PriceYuan == 0 {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clipRunes(value string, limit int) string {
	if value == "" || limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func shortDate(value string) string {
	trimmed := strings.TrimSpace(value)
	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return trimmed
	}
	return fmt.Sprintf("%d/%d", parsed.Month(), parsed.Day())
}

func nightsBetween(checkIn, checkOut string) int {
	in, err := time.Parse("2006-01-02", strings.TrimSpace(checkIn))
	if err != nil {
		return 0
	}
	out, err := time.Parse("2006-01-02", strings.TrimSpace(checkOut))
	if err != nil {
		return 0
	}
	nights := int(out.Sub(in).Hours() / 24)
	if nights <= 0 {
		return 0
	}
	return nights
}

func joinParts(parts []string) string {
	trimmed := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return strings.Join(trimmed, " · ")
}

func (t *HotelTool) QuerySummary(input json.RawMessage) string {
	var in SearchHotelsInput
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	parts := make([]string, 0, 6)
	parts = append(parts, in.City)
	if in.CheckIn != "" || in.CheckOut != "" {
		checkIn, checkOut := shortDate(in.CheckIn), shortDate(in.CheckOut)
		switch {
		case checkIn != "" && checkOut != "":
			parts = append(parts, checkIn+"–"+checkOut)
		case checkIn != "":
			parts = append(parts, checkIn+" 起")
		default:
			parts = append(parts, checkOut+" 前")
		}
	}
	if nights := nightsBetween(in.CheckIn, in.CheckOut); nights > 0 {
		parts = append(parts, fmt.Sprintf("%d 晚", nights))
	}
	if in.Keyword != "" {
		parts = append(parts, in.Keyword)
	}
	if in.MinStar > 0 {
		parts = append(parts, fmt.Sprintf("%d 星以上", in.MinStar))
	}
	if in.MaxPrice > 0 {
		parts = append(parts, fmt.Sprintf("每晚 ≤¥%d", in.MaxPrice))
	}
	return joinParts(parts)
}

func (t *HotelTool) ResultPreview(output json.RawMessage) []EventPreview {
	var decoded struct {
		Hotels []domain.Hotel `json:"hotels"`
	}
	if json.Unmarshal(output, &decoded) != nil {
		return nil
	}
	previews := make([]EventPreview, 0, previewLimit)
	for _, hotel := range decoded.Hotels {
		subtitleParts := make([]string, 0, 2)
		if hotel.Star > 0 {
			subtitleParts = append(subtitleParts, fmt.Sprintf("%d 星", hotel.Star))
		}
		if hotel.Score != nil {
			subtitleParts = append(subtitleParts, fmt.Sprintf("%.1f 分", *hotel.Score))
		}
		previews = append(previews, EventPreview{
			Title:     hotel.Name,
			Subtitle:  strings.Join(subtitleParts, " · "),
			PriceYuan: hotel.PricePerNightYuan,
		})
		if len(previews) == previewLimit {
			break
		}
	}
	return previews
}

func (t *GuideTool) QuerySummary(input json.RawMessage) string {
	var in SearchNotesInput
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	in.applyDefaults()
	parts := []string{strings.TrimSpace(in.Query)}
	if site := strings.TrimSpace(in.Site); site != "" {
		parts = append(parts, site)
	}
	return joinParts(parts)
}

func (t *GuideTool) ResultPreview(output json.RawMessage) []EventPreview {
	var decoded struct {
		Results []guideResult `json:"results"`
	}
	if json.Unmarshal(output, &decoded) != nil {
		return nil
	}
	previews := make([]EventPreview, 0, previewLimit)
	for _, result := range decoded.Results {
		subtitle := strings.TrimSpace(result.SiteName)
		if subtitle == "" {
			subtitle = "网页"
		}
		previews = append(previews, EventPreview{
			Title:    result.Title,
			Subtitle: subtitle,
		})
		if len(previews) == previewLimit {
			break
		}
	}
	return previews
}

func (t *WeatherTool) QuerySummary(input json.RawMessage) string {
	var in WeatherInput
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	parts := make([]string, 0, 3)
	parts = append(parts, in.City)
	if in.StartDate != "" {
		parts = append(parts, shortDate(in.StartDate)+" 起")
	}
	if in.Days > 0 {
		parts = append(parts, fmt.Sprintf("%d 天", in.Days))
	}
	return joinParts(parts)
}

func (t *WeatherTool) ResultPreview(output json.RawMessage) []EventPreview {
	var decoded struct {
		Days []domain.WeatherDay `json:"days"`
	}
	if json.Unmarshal(output, &decoded) != nil {
		return nil
	}
	previews := make([]EventPreview, 0, previewLimit)
	for _, day := range decoded.Days {
		previews = append(previews, EventPreview{
			Title:    shortDate(day.Date),
			Subtitle: weatherSummary(day),
		})
		if len(previews) == previewLimit {
			break
		}
	}
	return previews
}

func weatherSummary(day domain.WeatherDay) string {
	weather := ""
	switch {
	case day.DayWeather != "" && day.NightWeather != "" && day.DayWeather != day.NightWeather:
		weather = day.DayWeather + " / " + day.NightWeather
	case day.DayWeather != "":
		weather = day.DayWeather
	default:
		weather = day.NightWeather
	}
	temperature := ""
	switch {
	case day.DayTemp != "" && day.NightTemp != "":
		temperature = day.DayTemp + "~" + day.NightTemp + "°C"
	case day.DayTemp != "":
		temperature = day.DayTemp + "°C"
	case day.NightTemp != "":
		temperature = day.NightTemp + "°C"
	}
	return joinParts([]string{weather, temperature})
}
