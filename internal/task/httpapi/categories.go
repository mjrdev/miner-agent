package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task"
)

type categoryResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toCategoryResponse(c task.ProjectCategory) categoryResponse {
	return categoryResponse{
		ID:          c.ID,
		Name:        c.Name,
		Slug:        c.Slug,
		Description: c.Description,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

type createCategoryRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

func (h *Tasks) createCategory(w http.ResponseWriter, r *http.Request) {
	var req createCategoryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), task.CategoryInput{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
	})
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCategoryResponse(c))
}

type categoryListResponse struct {
	Items []categoryResponse `json:"items"`
}

func (h *Tasks) listCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.svc.ListCategories(r.Context())
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	items := make([]categoryResponse, 0, len(categories))
	for _, c := range categories {
		items = append(items, toCategoryResponse(c))
	}
	writeJSON(w, http.StatusOK, categoryListResponse{Items: items})
}

func (h *Tasks) getCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := h.svc.GetCategory(r.Context(), id)
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, toCategoryResponse(c))
}

type updateCategoryRequest struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Description *string `json:"description"`
}

func (h *Tasks) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateCategoryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := h.svc.UpdateCategory(r.Context(), id, task.CategoryPatch{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
	})
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, toCategoryResponse(c))
}

func (h *Tasks) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteCategory(r.Context(), id); err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
