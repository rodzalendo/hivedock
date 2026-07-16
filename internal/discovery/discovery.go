// Package discovery turns the stacks truth model into homepage cards with zero
// config: it resolves each user-facing service's name/group/url/icon/hidden via
// the priority chains in docs/ARCHITECTURE.md §3 (hivedock.* label →
// homepage.* label → automatic heuristic). Pure and deterministic; the icon
// matcher (icon.go) turns the resolved slug into an asset.
package discovery

import (
	"sort"

	"github.com/rogalinski/hivedock/internal/stacks"
)

// PortLink is one published-port destination for a card.
type PortLink struct {
	Label string `json:"label"` // e.g. "8096/tcp"
	URL   string `json:"url"`
}

// Entry is a resolved homepage card (one per user-facing service).
type Entry struct {
	Stack   string `json:"stack"`
	Service string `json:"service"`
	Name    string `json:"name"`
	Group   string `json:"group"`
	// ExplicitGroup marks a group that came from a hivedock.group/homepage.group
	// label (vs. the stack-name fallback). The dashboard only honors explicit
	// groups by default; derived ones collapse into its default group.
	ExplicitGroup bool       `json:"explicitGroup,omitempty"`
	URL           string     `json:"url,omitempty"`
	Ports         []PortLink `json:"ports,omitempty"`
	IconSlug      string     `json:"iconSlug,omitempty"`  // normalized image slug (icon matcher resolves it)
	StackSlug     string     `json:"stackSlug,omitempty"` // normalized stack name — icon fallback when the image slug has no asset
	Icon          string     `json:"icon,omitempty"`      // explicit icon label (user override or label) if set
	Description   string     `json:"description,omitempty"`
	Status        string     `json:"status"`           // running | stopped | ...
	Health        string     `json:"health,omitempty"` // healthy/unhealthy/starting ("" = no health check)
	Hidden        bool       `json:"hidden"`           // auto/label-hidden (UI may still reveal via toggle)
	// Sidecar marks a visible service that belongs under its stack's primary
	// card (e.g. immich-machine-learning under immich-server). The dashboard
	// rolls sidecars up behind the primary card's expander instead of giving
	// them their own tile. Only set when the stack has an identifiable primary.
	Sidecar bool `json:"sidecar,omitempty"`
}

// Options tune resolution.
type Options struct {
	// Host is the host:ip the browser reaches Hivedock at, used to build port
	// URLs (from the request Host header or a configured PUBLIC_HOST).
	Host string
	// HiddenOverride reports a user's explicit hide/unhide for a service, taking
	// precedence over the auto-hide heuristic. Returns (value, set).
	HiddenOverride func(stack, service string) (bool, bool)
	// IconOverride reports a user's custom icon (URL or slug) for a service,
	// taking precedence over any label. Returns (value, set).
	IconOverride func(stack, service string) (string, bool)
	// NameOverride reports a user's custom display name for a service, taking
	// precedence over labels and the automatic name. Returns (value, set).
	NameOverride func(stack, service string) (string, bool)
	// URLOverride reports a user's custom link URL for a service, taking
	// precedence over labels and the port heuristic (the reliable fallback for
	// host-network or shared-network services). Returns (value, set).
	URLOverride func(stack, service string) (string, bool)
}

// Resolve produces one entry per service across all stacks (managed + external).
func Resolve(all []stacks.Stack, opts Options) []Entry {
	var entries []Entry
	for _, st := range all {
		// Candidates are the non-hidden services; a single-candidate managed
		// stack gets the stack's name as its default card name.
		var visible []stacks.Service
		for _, svc := range st.Services {
			if !isHidden(st, svc, opts) {
				visible = append(visible, svc)
			}
		}
		primary := primaryService(st, visible)
		for _, svc := range st.Services {
			e := resolveOne(st, svc, len(visible), opts)
			if primary != "" && svc.Name != primary && !e.Hidden {
				e.Sidecar = true
			}
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Group != entries[j].Group {
			return entries[i].Group < entries[j].Group
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

func resolveOne(st stacks.Stack, svc stacks.Service, candidates int, opts Options) Entry {
	l := svc.Labels

	name := firstLabel(l, "hivedock.name", "homepage.name")
	if name == "" {
		if candidates == 1 && st.Origin == stacks.OriginManaged {
			name = humanize(st.Name)
		} else {
			name = humanize(svc.Name)
		}
	}
	// A user's rename wins over labels and the automatic name.
	if opts.NameOverride != nil {
		if v, set := opts.NameOverride(st.Name, svc.Name); set && v != "" {
			name = v
		}
	}

	group := firstLabel(l, "hivedock.group", "homepage.group")
	explicitGroup := group != ""
	if group == "" {
		group = humanize(st.Name)
	}

	url := firstLabel(l, "hivedock.url", "homepage.href", "homepage.url")
	var ports []PortLink
	if url == "" {
		// A service behind another's network (compose `network_mode:
		// service:X`, e.g. qBittorrent behind a gluetun VPN) publishes no
		// ports of its own — they live on the target. Borrow the target's
		// ports so the app card gets a link instead of the VPN sidecar.
		portSvc := svc
		if len(svc.Ports) == 0 && svc.NetworkFrom != "" {
			for _, sib := range st.Services {
				if sib.Name == svc.NetworkFrom && len(sib.Ports) > 0 {
					portSvc = sib
					break
				}
			}
		}
		url, ports = urlHeuristic(portSvc, opts.Host)
	}
	// A user's explicit link wins over labels and the heuristic — the reliable
	// fix when a service publishes no ports on its own container.
	if opts.URLOverride != nil {
		if v, set := opts.URLOverride(st.Name, svc.Name); set && v != "" {
			url = v
		}
	}

	// Icon: user override wins over any compose label.
	icon := firstLabel(l, "hivedock.icon", "homepage.icon")
	if opts.IconOverride != nil {
		if v, set := opts.IconOverride(st.Name, svc.Name); set {
			icon = v
		}
	}

	e := Entry{
		Stack:         st.Name,
		Service:       svc.Name,
		Name:          name,
		Group:         group,
		ExplicitGroup: explicitGroup,
		URL:           url,
		Ports:         ports,
		Icon:          icon,
		IconSlug:      normalizeImage(svc.Image),
		StackSlug:     normalizeImage(st.Name),
		Description:   firstLabel(l, "hivedock.description", "homepage.description"),
		Status:        svc.State,
		Health:        svc.Health,
		Hidden:        isHidden(st, svc, opts),
	}
	return e
}

// isHidden applies the hidden priority chain: explicit label → user override →
// auto-hide heuristic.
func isHidden(st stacks.Stack, svc stacks.Service, opts Options) bool {
	if v, ok := boolLabel(svc.Labels, "hivedock.hidden", "homepage.hidden"); ok {
		return v
	}
	if opts.HiddenOverride != nil {
		if v, set := opts.HiddenOverride(st.Name, svc.Name); set {
			return v
		}
	}
	return autoHide(svc)
}
