package model

// Game ports are expanded to individual registered endpoints, never an open
// transport or an unrestricted firewall rule. PortEnd=0 means a single port.
type GamePort struct {
	Protocol    string `json:"protocol"`
	Port        uint16 `json:"port"`
	PortEnd     uint16 `json:"port_end,omitempty"`
	Description string `json:"description,omitempty"`
}

type Game struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Summary       string     `json:"summary"`
	SourceURL     string     `json:"source_url"`
	CoverURL      string     `json:"cover_url"`
	BackgroundURL string     `json:"background_url"`
	Ports         []GamePort `json:"ports"`
	Enabled       bool       `json:"enabled"`
	Revision      int64      `json:"revision"`
}

type GameUpdateRequest struct {
	Name     string     `json:"name"`
	Ports    []GamePort `json:"ports"`
	Enabled  bool       `json:"enabled"`
	Revision int64      `json:"revision"`
}

type GameImportRequest struct {
	URL string `json:"url"`
}
