package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core): another
// Pocket app may act here for a person with a pcx_ token, only within these
// scopes. Every route not listed in linkRoutes stays closed to it. A link
// works on exactly what the person sees with their own login (their items and
// the ones shared with them), never changes who an item is shared with, and
// quantity changes it makes are movements whose note names the app.

var linkOffers = []core.Offer{
	{Scope: "items:read", Name: "See your items", Description: "Search your items and see how many are on hand, so the app can offer them (parts and fluids, for example)."},
	{Scope: "items:adjust", Name: "Take items off the shelf", Description: "Change how many of an item you have (a part used, a fluid topped up); every change is a movement with a note naming the app."},
	{Scope: "items:write", Name: "Add and edit items", Description: "Create items (e.g. groceries for your pantry), edit their details and archive them. Sharing an item with others stays with you."},
	{Scope: "activity:read", Name: "See your inventory activity", Description: "Read the history of what was added, used or moved, for your journal."},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/items$`), "items:read"},
	{"GET", regexp.MustCompile(`^/api/v1/items/\d+$`), "items:read"},
	{"GET", regexp.MustCompile(`^/api/v1/lookup$`), "items:read"},
	{"GET", regexp.MustCompile(`^/api/v1/locations$`), "items:read"},
	{"POST", regexp.MustCompile(`^/api/v1/items/\d+/adjust$`), "items:adjust"},
	{"POST", regexp.MustCompile(`^/api/v1/items$`), "items:write"},
	{"PUT", regexp.MustCompile(`^/api/v1/items/\d+$`), "items:write"},
	{"POST", regexp.MustCompile(`^/api/v1/items/\d+/archive$`), "items:write"},
	{"GET", regexp.MustCompile(`^/api/v1/activity$`), "activity:read"},
}

var errLinkScope = errors.New("outside the link's scope")

func linkAllows(scopes []string, r *http.Request) bool {
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token and maps it to the person it acts for. The
// second value names the acting app (its Core name, else its slug).
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	app := in.App.Name
	if app == "" {
		app = in.App.Slug
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, app, err
}

const linkAppKey ctxKey = 2

// linkApp returns the acting app's name when the request came in on a link
// token, "" otherwise.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
