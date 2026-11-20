package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"trippilot/internal/domain"
	"trippilot/internal/repo"
)

type shareResponse struct {
	Slug       string    `json:"slug"`
	URL        string    `json:"url"`
	TripID     string    `json:"trip_id"`
	VersionSeq int32     `json:"version_seq"`
	CreatedAt  time.Time `json:"created_at"`
}

type sharedTripResponse struct {
	Slug      string           `json:"slug"`
	TripID    string           `json:"trip_id"`
	CreatedAt time.Time        `json:"created_at"`
	Version   repo.PlanVersion `json:"version"`
}

func handleCreateShare(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tripID := chi.URLParam(r, "tripID")
		if _, err := deps.Store.GetTrip(r.Context(), tripID); err != nil {
			WriteError(w, r, mapStoreError(err))
			return
		}
		versions, err := deps.Store.ListPlanVersions(r.Context(), tripID)
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		var final *repo.PlanVersion
		for index := range versions {
			if versions[index].Status == "accepted" {
				final = &versions[index]
				break
			}
		}
		if final == nil {
			WriteError(w, r, &Error{Status: http.StatusConflict, Code: "no_final_version", Message: "行程尚未确认，不能分享"})
			return
		}

		link, err := deps.Store.CreateShareLink(r.Context(), tripID, final.ID)
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		writeJSON(w, http.StatusCreated, shareResponse{
			Slug:       link.Slug,
			URL:        shareURL(link.Slug),
			TripID:     link.TripID,
			VersionSeq: final.Seq,
			CreatedAt:  link.CreatedAt,
		})
	}
}

func handleGetShare(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		link, err := deps.Store.GetShareLink(r.Context(), chi.URLParam(r, "slug"))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				WriteError(w, r, &Error{Status: http.StatusNotFound, Code: "share_not_found", Message: "分享链接不存在"})
				return
			}
			WriteError(w, r, errInternal(err))
			return
		}
		version, err := deps.Store.GetPlanVersion(r.Context(), link.VersionID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				WriteError(w, r, &Error{Status: http.StatusNotFound, Code: "version_not_found", Message: "分享的行程不存在"})
				return
			}
			WriteError(w, r, errInternal(err))
			return
		}
		version.Research = sanitizeSharedResearch(version.Research)
		writeJSON(w, http.StatusOK, sharedTripResponse{
			Slug:      link.Slug,
			TripID:    link.TripID,
			CreatedAt: link.CreatedAt,
			Version:   version,
		})
	}
}

func shareURL(slug string) string {
	return "/api/share/" + slug
}

// sharedResearch is the public slice of a version's research. Tool metrics,
// hotel provider IDs and sidecar handles never leave the server.
type sharedResearch struct {
	Accommodation *sharedAccommodation `json:"accommodation,omitempty"`
	Guides        []domain.GuideSignal `json:"guides,omitempty"`
	Notes         []string             `json:"notes,omitempty"`
	Warnings      []string             `json:"warnings,omitempty"`
}

// sharedAccommodation and sharedHotel mirror the public research fields while
// dropping provider IDs, booking links and fetch timestamps.
type sharedAccommodation struct {
	Mentioned       bool          `json:"mentioned"`
	Specified       []string      `json:"specified,omitempty"`
	Selected        *sharedHotel  `json:"selected,omitempty"`
	Options         []sharedHotel `json:"options,omitempty"`
	AnchorDays      []int         `json:"anchor_days,omitempty"`
	Nights          int           `json:"nights"`
	NightlyYuan     int           `json:"nightly_yuan"`
	TotalYuan       int           `json:"total_yuan"`
	PriceEstimated  bool          `json:"price_estimated"`
	IncludeInBudget bool          `json:"include_in_budget"`
	Reason          string        `json:"reason,omitempty"`
}

type sharedHotel struct {
	Name              string   `json:"name"`
	Address           string   `json:"address,omitempty"`
	City              string   `json:"city,omitempty"`
	Lat               float64  `json:"lat"`
	Lng               float64  `json:"lng"`
	Star              int      `json:"star,omitempty"`
	Score             *float64 `json:"score,omitempty"`
	PricePerNightYuan int      `json:"price_per_night_yuan"`
	PriceEstimated    bool     `json:"price_estimated"`
}

func sanitizeSharedResearch(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}")) {
		return json.RawMessage(`{}`)
	}
	var brief domain.ResearchBrief
	if err := json.Unmarshal(trimmed, &brief); err != nil {
		return json.RawMessage(`{}`)
	}
	shared := sharedResearch{
		Guides:   sharedGuides(brief.Guides),
		Notes:    append([]string(nil), brief.Notes...),
		Warnings: append([]string(nil), brief.Warnings...),
	}
	shared.Accommodation = sharedAccommodationOf(brief.Accommodation)
	encoded, err := json.Marshal(shared)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func sharedAccommodationOf(brief *domain.AccommodationBrief) *sharedAccommodation {
	if brief == nil {
		return nil
	}
	out := sharedAccommodation{
		Mentioned:       brief.Mentioned,
		Specified:       append([]string(nil), brief.Specified...),
		Selected:        sharedHotelPtr(brief.Selected),
		AnchorDays:      append([]int(nil), brief.AnchorDays...),
		Nights:          brief.Nights,
		NightlyYuan:     brief.NightlyYuan,
		TotalYuan:       brief.TotalYuan,
		PriceEstimated:  brief.PriceEstimated,
		IncludeInBudget: brief.IncludeInBudget,
		Reason:          brief.Reason,
	}
	out.Options = make([]sharedHotel, 0, len(brief.Options))
	for _, hotel := range brief.Options {
		out.Options = append(out.Options, sharedHotelOf(hotel))
	}
	return &out
}

func sharedHotelPtr(hotel *domain.Hotel) *sharedHotel {
	if hotel == nil {
		return nil
	}
	sanitized := sharedHotelOf(*hotel)
	return &sanitized
}

func sharedHotelOf(hotel domain.Hotel) sharedHotel {
	return sharedHotel{
		Name:              hotel.Name,
		Address:           hotel.Address,
		City:              hotel.City,
		Lat:               hotel.Lat,
		Lng:               hotel.Lng,
		Star:              hotel.Star,
		Score:             hotel.Score,
		PricePerNightYuan: hotel.PricePerNightYuan,
		PriceEstimated:    hotel.PriceEstimated,
	}
}

func sharedGuides(guides []domain.GuideSignal) []domain.GuideSignal {
	if len(guides) == 0 {
		return nil
	}
	out := make([]domain.GuideSignal, 0, len(guides))
	for _, guide := range guides {
		out = append(out, domain.GuideSignal{
			POIID:     guide.POIID,
			Weight:    guide.Weight,
			Mentions:  guide.Mentions,
			Note:      guide.Note,
			Citations: append([]domain.GuideCitation(nil), guide.Citations...),
		})
	}
	return out
}
