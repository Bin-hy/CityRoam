package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

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
