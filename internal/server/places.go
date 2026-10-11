package server

import (
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

// ---- locations ----

func (s *Server) handleListLocations(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.ListLocations(r.Context())
	if err != nil {
		s.fail(w, err, "list locations")
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleCreateLocation(w http.ResponseWriter, r *http.Request) {
	var in store.Location
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	l := &store.Location{Name: in.Name, Description: strings.TrimSpace(in.Description)}
	if err := s.store.CreateLocation(r.Context(), l); err != nil {
		s.fail(w, err, "create location")
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) handleUpdateLocation(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.GetLocation(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get location")
		return
	}
	var in store.Location
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	l.Name, l.Description = in.Name, strings.TrimSpace(in.Description)
	if err := s.store.UpdateLocation(r.Context(), l); err != nil {
		s.fail(w, err, "update location")
		return
	}
	l, _ = s.store.GetLocation(r.Context(), l.ID)
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleDeleteLocation(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteLocation(r.Context(), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete location")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReorderLocations(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.ReorderLocations(r.Context(), in.IDs); err != nil {
		s.fail(w, err, "reorder locations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- categories ----

func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.ListCategories(r.Context())
	if err != nil {
		s.fail(w, err, "list categories")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	var in store.Category
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	c := &store.Category{Name: in.Name, Color: in.Color}
	if err := s.store.CreateCategory(r.Context(), c); err != nil {
		s.fail(w, err, "create category")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.GetCategory(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get category")
		return
	}
	var in store.Category
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	c.Name, c.Color = in.Name, in.Color
	if err := s.store.UpdateCategory(r.Context(), c); err != nil {
		s.fail(w, err, "update category")
		return
	}
	c, _ = s.store.GetCategory(r.Context(), c.ID)
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteCategory(r.Context(), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete category")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReorderCategories(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.ReorderCategories(r.Context(), in.IDs); err != nil {
		s.fail(w, err, "reorder categories")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- tags ----

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.ListTags(r.Context())
	if err != nil {
		s.fail(w, err, "list tags")
		return
	}
	writeJSON(w, http.StatusOK, t)
}
