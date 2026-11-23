package twitch

import "time"

// DefaultClientID is the default public Twitch Application Client ID for Device Code Flow.
// It can be overridden at build time via:
//
//	-ldflags "-X github.com/mrpoundsign/PoundControl-Twitch/internal/twitch.DefaultClientID=<id>"
//
// Users can also override it in Settings -> Twitch.
var DefaultClientID = "ihtyx8njcjifiv7lg2oxjmk49s0uxz"

// PluginVersion can be overridden at build time via -ldflags.
var PluginVersion = "0.1.0"

// HTTPRequest mirrors PoundControl's GuestHTTPRequest ABI.
type HTTPRequest struct {
	Method    string            `json:"method,omitempty"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	TimeoutMs int               `json:"timeoutMs,omitempty"`
}

// HTTPResponse mirrors PoundControl's GuestHTTPResponse ABI.
type HTTPResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// VisualPatch mirrors PoundControl's plugin.VisualPatch ABI.
type VisualPatch struct {
	Title    *string        `json:"title,omitempty"`
	Subtitle *string        `json:"subtitle,omitempty"`
	Color    *string        `json:"color,omitempty"`
	ImageURI *string        `json:"image,omitempty"`
	State    *int           `json:"state,omitempty"`
	Feedback map[string]any `json:"feedback,omitempty"`
	Flash    string         `json:"flash,omitempty"` // "", "ok", or "alert"
}

// ActionEvent mirrors PoundControl's plugin.ActionEvent ABI.
type ActionEvent struct {
	Type       string         `json:"event"`
	Context    string         `json:"context"`
	ActionUUID string         `json:"action"`
	Controller string         `json:"controller,omitempty"`
	Settings   map[string]any `json:"settings,omitempty"`
	State      int            `json:"state,omitempty"`
	Ticks      int            `json:"ticks,omitempty"`
}

// TimerEvent mirrors PoundControl's GuestTimerEvent ABI.
type TimerEvent struct {
	TimerID string `json:"timerId"`
	Context string `json:"context"`
}

// ConfigActionEvent mirrors PoundControl's GuestConfigActionEvent ABI.
type ConfigActionEvent struct {
	Action   string         `json:"action"`
	Settings map[string]any `json:"settings,omitempty"`
}

// Host abstracts the PoundControl Wasm host imports so the entire plugin is unit-testable in native Go.
type Host interface {
	UpdateVisual(ctxID string, patch VisualPatch)
	SaveSettings(ctxID string, settings map[string]any) error
	GetGlobalSettings() map[string]any
	SaveGlobalSettings(settings map[string]any) error
	Log(msg string)
	HTTPRequest(req HTTPRequest) HTTPResponse
	SetTimer(timerID, ctxID string, intervalMs uint32) error
	Now() time.Time
}
