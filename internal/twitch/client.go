package twitch

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	twitchAuthBase  = "https://id.twitch.tv"
	twitchHelixBase = "https://api.twitch.tv"

	deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

	// All Twitch OAuth scopes required for full Elgato Stream Deck Twitch parity.
	requiredScopes = "channel:manage:broadcast channel:edit:commercial clips:edit user:write:chat moderator:manage:chat_messages moderator:manage:chat_settings moderator:manage:shield_mode"

	httpTimeoutMs = 3500
	expirySkewSec = 60
)

var (
	ErrNotAuthenticated = errors.New("not signed in to Twitch")
	ErrMissingClientID  = errors.New("twitch client ID is not configured")
	ErrAuthPending      = errors.New("authorization pending")
	ErrSlowDown         = errors.New("slow down polling")
	ErrDeviceExpired    = errors.New("device code expired or denied")
)

// APIError captures a structured error response from Twitch OAuth or Helix APIs.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("twitch api %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("twitch api %d", e.Status)
}

// AuthState holds persisted OAuth credentials and user identity saved in plugin global settings.
type AuthState struct {
	ClientID     string `json:"clientId,omitempty"`
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
	UserID       string `json:"userId,omitempty"`
	UserLogin    string `json:"userLogin,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
}

func (a AuthState) IsSignedIn() bool {
	return strings.TrimSpace(a.AccessToken) != "" && strings.TrimSpace(a.ClientID) != ""
}

// DeviceAuthResponse is returned by POST https://id.twitch.tv/oauth2/device.
type DeviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
}

// TokenResponse is returned by POST https://id.twitch.tv/oauth2/token.
type TokenResponse struct {
	AccessToken  string   `json:"access_token"`
	ExpiresIn    int      `json:"expires_in"`
	RefreshToken string   `json:"refresh_token"`
	Scope        []string `json:"scope"`
	TokenType    string   `json:"token_type"`
}

// UserInfo represents a Twitch user from GET /helix/users.
type UserInfo struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"display_name"`
}

// ChatSettings represents live moderation flags from /helix/chat/settings.
type ChatSettings struct {
	EmoteMode            bool `json:"emote_mode"`
	FollowerMode         bool `json:"follower_mode"`
	FollowerModeDuration *int `json:"follower_mode_duration"`
	SlowMode             bool `json:"slow_mode"`
	SlowModeWaitTime     *int `json:"slow_mode_wait_time"`
	SubscriberMode       bool `json:"subscriber_mode"`
}

// ChatSettingsPatch carries partial updates for PATCH /helix/chat/settings.
type ChatSettingsPatch struct {
	EmoteMode            *bool `json:"emote_mode,omitempty"`
	FollowerMode         *bool `json:"follower_mode,omitempty"`
	FollowerModeDuration *int  `json:"follower_mode_duration,omitempty"`
	SlowMode             *bool `json:"slow_mode,omitempty"`
	SlowModeWaitTime     *int  `json:"slow_mode_wait_time,omitempty"`
	SubscriberMode       *bool `json:"subscriber_mode,omitempty"`
}

// ChannelInfoPatch carries updates for PATCH /helix/channels.
type ChannelInfoPatch struct {
	Title               string   `json:"title,omitempty"`
	GameID              string   `json:"game_id,omitempty"`
	BroadcasterLanguage string   `json:"broadcaster_language,omitempty"`
	Tags                []string `json:"tags,omitempty"`
}

// ClipResult represents a created clip from POST /helix/clips.
type ClipResult struct {
	ID      string `json:"id"`
	EditURL string `json:"edit_url"`
}

// MarkerResult represents a created stream marker from POST /helix/streams/markers.
type MarkerResult struct {
	ID              string `json:"id"`
	PositionSeconds int    `json:"position_seconds"`
	Description     string `json:"description"`
}

// CommercialResult represents a commercial response from POST /helix/channels/commercial.
type CommercialResult struct {
	Length     int    `json:"length"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retry_after"`
}

// StreamInfo represents live stream data from GET /helix/streams.
type StreamInfo struct {
	Live        bool
	ViewerCount int
	Title       string
	GameName    string
}

// Client wraps Twitch OAuth2 Device Code Flow and Helix REST API calls over the sandboxed Host.
type Client struct {
	host       Host
	auth       AuthState
	onAuthSave func(AuthState)

	userIDCache map[string]string // lower(login) -> user_id
	gameIDCache map[string]string // lower(name) -> game_id
}

// NewClient creates a Twitch API client backed by the given Host.
func NewClient(host Host, initial AuthState, onAuthSave func(AuthState)) *Client {
	return &Client{
		host:        host,
		auth:        initial,
		onAuthSave:  onAuthSave,
		userIDCache: make(map[string]string),
		gameIDCache: make(map[string]string),
	}
}

func (c *Client) Auth() AuthState {
	return c.auth
}

func (c *Client) SetAuth(a AuthState) {
	c.auth = a
	if c.onAuthSave != nil {
		c.onAuthSave(a)
	}
}

func (c *Client) SetClientID(clientID string) {
	id := strings.TrimSpace(clientID)
	if id == "" || id == c.auth.ClientID {
		return
	}
	c.auth.ClientID = id
	if c.onAuthSave != nil {
		c.onAuthSave(c.auth)
	}
}

func (c *Client) EffectiveClientID() string {
	if id := strings.TrimSpace(c.auth.ClientID); id != "" {
		return id
	}
	return strings.TrimSpace(DefaultClientID)
}

// StartDeviceAuth initiates Twitch OAuth2 Device Code Grant Flow.
func (c *Client) StartDeviceAuth(clientID string) (DeviceAuthResponse, error) {
	id := strings.TrimSpace(clientID)
	if id == "" {
		id = c.EffectiveClientID()
	}
	if id == "" {
		return DeviceAuthResponse{}, ErrMissingClientID
	}

	form := url.Values{}
	form.Set("client_id", id)
	form.Set("scopes", requiredScopes)

	resp := c.host.HTTPRequest(HTTPRequest{
		Method:    "POST",
		URL:       twitchAuthBase + "/oauth2/device",
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Body:      form.Encode(),
		TimeoutMs: httpTimeoutMs,
	})
	if err := checkHTTPTransport(resp); err != nil {
		return DeviceAuthResponse{}, err
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return DeviceAuthResponse{}, parseAPIError(resp)
	}

	var out DeviceAuthResponse
	if err := json.Unmarshal([]byte(resp.Body), &out); err != nil {
		return DeviceAuthResponse{}, fmt.Errorf("decode device auth response: %w", err)
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	if out.VerificationURI == "" {
		out.VerificationURI = "https://www.twitch.tv/activate"
	}
	return out, nil
}

// PollDeviceToken checks whether the user has completed Device Code authorization.
func (c *Client) PollDeviceToken(clientID, deviceCode string) (TokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scopes", requiredScopes)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", deviceGrantType)

	resp := c.host.HTTPRequest(HTTPRequest{
		Method:    "POST",
		URL:       twitchAuthBase + "/oauth2/token",
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Body:      form.Encode(),
		TimeoutMs: httpTimeoutMs,
	})
	if err := checkHTTPTransport(resp); err != nil {
		return TokenResponse{}, err
	}
	if resp.Status >= 200 && resp.Status < 300 {
		var tok TokenResponse
		if err := json.Unmarshal([]byte(resp.Body), &tok); err != nil {
			return TokenResponse{}, fmt.Errorf("decode token response: %w", err)
		}
		return tok, nil
	}

	apiErr := parseAPIError(resp)
	msgLower := strings.ToLower(apiErr.Message)
	switch {
	case strings.Contains(msgLower, "authorization_pending"):
		return TokenResponse{}, ErrAuthPending
	case strings.Contains(msgLower, "slow_down"):
		return TokenResponse{}, ErrSlowDown
	case strings.Contains(msgLower, "expired"),
		strings.Contains(msgLower, "invalid device code"),
		strings.Contains(msgLower, "access_denied"):
		return TokenResponse{}, ErrDeviceExpired
	default:
		return TokenResponse{}, apiErr
	}
}

// RefreshAccessToken exchanges the stored refresh token for a fresh access token.
func (c *Client) RefreshAccessToken() error {
	clientID := c.EffectiveClientID()
	if clientID == "" {
		return ErrMissingClientID
	}
	if strings.TrimSpace(c.auth.RefreshToken) == "" {
		return ErrNotAuthenticated
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", c.auth.RefreshToken)

	resp := c.host.HTTPRequest(HTTPRequest{
		Method:    "POST",
		URL:       twitchAuthBase + "/oauth2/token",
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Body:      form.Encode(),
		TimeoutMs: httpTimeoutMs,
	})
	if err := checkHTTPTransport(resp); err != nil {
		return err
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return parseAPIError(resp)
	}

	var tok TokenResponse
	if err := json.Unmarshal([]byte(resp.Body), &tok); err != nil {
		return fmt.Errorf("decode refresh token response: %w", err)
	}
	c.auth.ClientID = clientID
	c.auth.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		c.auth.RefreshToken = tok.RefreshToken
	}
	if tok.ExpiresIn > 0 {
		c.auth.ExpiresAt = c.host.Now().Unix() + int64(tok.ExpiresIn)
	}
	if c.onAuthSave != nil {
		c.onAuthSave(c.auth)
	}
	return nil
}

// RevokeToken attempts a best-effort token revocation and clears local credentials while preserving ClientID.
func (c *Client) RevokeToken() {
	clientID := c.EffectiveClientID()
	token := strings.TrimSpace(c.auth.AccessToken)
	if clientID != "" && token != "" {
		form := url.Values{}
		form.Set("client_id", clientID)
		form.Set("token", token)
		_ = c.host.HTTPRequest(HTTPRequest{
			Method:    "POST",
			URL:       twitchAuthBase + "/oauth2/revoke",
			Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			Body:      form.Encode(),
			TimeoutMs: httpTimeoutMs,
		})
	}
	c.SetAuth(AuthState{ClientID: clientID})
}

func (c *Client) ensureToken() error {
	if !c.auth.IsSignedIn() {
		return ErrNotAuthenticated
	}
	if c.auth.ExpiresAt > 0 && c.host.Now().Unix() >= c.auth.ExpiresAt-expirySkewSec {
		if c.auth.RefreshToken != "" {
			if err := c.RefreshAccessToken(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) helixRequest(method, path string, body any) (HTTPResponse, error) {
	if err := c.ensureToken(); err != nil {
		return HTTPResponse{}, err
	}

	var bodyStr string
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return HTTPResponse{}, err
		}
		bodyStr = string(raw)
	}

	doReq := func() HTTPResponse {
		hdrs := map[string]string{
			"Authorization": "Bearer " + c.auth.AccessToken,
			"Client-Id":     c.EffectiveClientID(),
		}
		if bodyStr != "" {
			hdrs["Content-Type"] = "application/json"
		}
		return c.host.HTTPRequest(HTTPRequest{
			Method:    method,
			URL:       twitchHelixBase + path,
			Headers:   hdrs,
			Body:      bodyStr,
			TimeoutMs: httpTimeoutMs,
		})
	}

	resp := doReq()
	if err := checkHTTPTransport(resp); err != nil {
		return resp, err
	}

	// Retry once after refreshing token on 401 Unauthorized.
	if resp.Status == 401 && c.auth.RefreshToken != "" {
		if err := c.RefreshAccessToken(); err == nil {
			resp = doReq()
			if err := checkHTTPTransport(resp); err != nil {
				return resp, err
			}
		}
	}

	if resp.Status < 200 || resp.Status >= 300 {
		return resp, parseAPIError(resp)
	}
	return resp, nil
}

// GetCurrentUser fetches the signed-in Twitch user and caches identity in AuthState.
func (c *Client) GetCurrentUser() (UserInfo, error) {
	resp, err := c.helixRequest("GET", "/helix/users", nil)
	if err != nil {
		return UserInfo{}, err
	}
	var payload struct {
		Data []UserInfo `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return UserInfo{}, fmt.Errorf("decode user info: %w", err)
	}
	if len(payload.Data) == 0 {
		return UserInfo{}, errors.New("twitch returned no user info")
	}
	u := payload.Data[0]
	c.auth.UserID = u.ID
	c.auth.UserLogin = u.Login
	c.auth.DisplayName = u.DisplayName
	if c.onAuthSave != nil {
		c.onAuthSave(c.auth)
	}
	c.userIDCache[strings.ToLower(u.Login)] = u.ID
	return u, nil
}

// EnsureUserID returns the signed-in user's ID, fetching it if not yet cached.
func (c *Client) EnsureUserID() (string, error) {
	if strings.TrimSpace(c.auth.UserID) != "" {
		return c.auth.UserID, nil
	}
	u, err := c.GetCurrentUser()
	if err != nil {
		return "", err
	}
	return u.ID, nil
}

// ResolveBroadcasterID resolves an optional channel login name to a broadcaster ID (defaults to signed-in user).
func (c *Client) ResolveBroadcasterID(channelLogin string) (string, error) {
	login := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(channelLogin), "@"))
	if login == "" || strings.EqualFold(login, c.auth.UserLogin) {
		return c.EnsureUserID()
	}
	if id, ok := c.userIDCache[login]; ok {
		return id, nil
	}

	resp, err := c.helixRequest("GET", "/helix/users?login="+url.QueryEscape(login), nil)
	if err != nil {
		return "", err
	}
	var payload struct {
		Data []UserInfo `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return "", fmt.Errorf("decode channel lookup: %w", err)
	}
	if len(payload.Data) == 0 {
		return "", fmt.Errorf("channel %q not found", login)
	}
	id := payload.Data[0].ID
	c.userIDCache[login] = id
	return id, nil
}

// ResolveGameID looks up a Twitch category/game ID by name, falling back to category search.
func (c *Client) ResolveGameID(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", nil
	}
	lower := strings.ToLower(trimmed)
	if lower == "none" || lower == "0" {
		return "0", nil
	}
	if id, ok := c.gameIDCache[lower]; ok {
		return id, nil
	}

	// Try exact game name first.
	resp, err := c.helixRequest("GET", "/helix/games?name="+url.QueryEscape(trimmed), nil)
	if err != nil {
		return "", err
	}
	var exact struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &exact); err == nil && len(exact.Data) > 0 {
		c.gameIDCache[lower] = exact.Data[0].ID
		return exact.Data[0].ID, nil
	}

	// Fallback to fuzzy category search.
	searchResp, err := c.helixRequest("GET", "/helix/search/categories?query="+url.QueryEscape(trimmed)+"&first=1", nil)
	if err != nil {
		return "", err
	}
	var search struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(searchResp.Body), &search); err != nil {
		return "", fmt.Errorf("decode category search: %w", err)
	}
	if len(search.Data) == 0 {
		return "", fmt.Errorf("category %q not found", trimmed)
	}
	c.gameIDCache[lower] = search.Data[0].ID
	return search.Data[0].ID, nil
}

// ModifyChannelInfo updates stream title, category, language, or tags.
func (c *Client) ModifyChannelInfo(patch ChannelInfoPatch) error {
	broadcasterID, err := c.EnsureUserID()
	if err != nil {
		return err
	}
	_, err = c.helixRequest("PATCH", "/helix/channels?broadcaster_id="+url.QueryEscape(broadcasterID), patch)
	return err
}

// SendChatMessage posts a chat message to the specified broadcaster's chat.
func (c *Client) SendChatMessage(broadcasterID, message string) error {
	senderID, err := c.EnsureUserID()
	if err != nil {
		return err
	}
	body := map[string]string{
		"broadcaster_id": broadcasterID,
		"sender_id":      senderID,
		"message":        message,
	}
	resp, err := c.helixRequest("POST", "/helix/chat/messages", body)
	if err != nil {
		return err
	}
	var payload struct {
		Data []struct {
			IsSent     bool `json:"is_sent"`
			DropReason *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"drop_reason"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err == nil && len(payload.Data) > 0 {
		if !payload.Data[0].IsSent && payload.Data[0].DropReason != nil {
			return fmt.Errorf("message dropped: %s", payload.Data[0].DropReason.Message)
		}
	}
	return nil
}

// ClearChat deletes all messages in the target broadcaster's chat.
func (c *Client) ClearChat(broadcasterID string) error {
	modID, err := c.EnsureUserID()
	if err != nil {
		return err
	}
	q := url.Values{}
	q.Set("broadcaster_id", broadcasterID)
	q.Set("moderator_id", modID)
	_, err = c.helixRequest("DELETE", "/helix/moderation/chat?"+q.Encode(), nil)
	return err
}

// GetChatSettings retrieves current chat moderation settings.
func (c *Client) GetChatSettings(broadcasterID string) (ChatSettings, error) {
	modID, err := c.EnsureUserID()
	if err != nil {
		return ChatSettings{}, err
	}
	q := url.Values{}
	q.Set("broadcaster_id", broadcasterID)
	q.Set("moderator_id", modID)
	resp, err := c.helixRequest("GET", "/helix/chat/settings?"+q.Encode(), nil)
	if err != nil {
		return ChatSettings{}, err
	}
	var payload struct {
		Data []ChatSettings `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return ChatSettings{}, fmt.Errorf("decode chat settings: %w", err)
	}
	if len(payload.Data) == 0 {
		return ChatSettings{}, errors.New("empty chat settings response")
	}
	return payload.Data[0], nil
}

// UpdateChatSettings patches chat moderation settings and returns the updated state.
func (c *Client) UpdateChatSettings(broadcasterID string, patch ChatSettingsPatch) (ChatSettings, error) {
	modID, err := c.EnsureUserID()
	if err != nil {
		return ChatSettings{}, err
	}
	q := url.Values{}
	q.Set("broadcaster_id", broadcasterID)
	q.Set("moderator_id", modID)
	resp, err := c.helixRequest("PATCH", "/helix/chat/settings?"+q.Encode(), patch)
	if err != nil {
		return ChatSettings{}, err
	}
	var payload struct {
		Data []ChatSettings `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return ChatSettings{}, fmt.Errorf("decode updated chat settings: %w", err)
	}
	if len(payload.Data) == 0 {
		return ChatSettings{}, errors.New("empty updated chat settings response")
	}
	return payload.Data[0], nil
}

// GetShieldMode retrieves whether Twitch Shield Mode is currently active.
func (c *Client) GetShieldMode(broadcasterID string) (bool, error) {
	modID, err := c.EnsureUserID()
	if err != nil {
		return false, err
	}
	q := url.Values{}
	q.Set("broadcaster_id", broadcasterID)
	q.Set("moderator_id", modID)
	resp, err := c.helixRequest("GET", "/helix/moderation/shield_mode?"+q.Encode(), nil)
	if err != nil {
		return false, err
	}
	var payload struct {
		Data []struct {
			IsActive bool `json:"is_active"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return false, fmt.Errorf("decode shield mode: %w", err)
	}
	if len(payload.Data) == 0 {
		return false, errors.New("empty shield mode response")
	}
	return payload.Data[0].IsActive, nil
}

// UpdateShieldMode enables or disables Twitch Shield Mode.
func (c *Client) UpdateShieldMode(broadcasterID string, active bool) (bool, error) {
	modID, err := c.EnsureUserID()
	if err != nil {
		return false, err
	}
	q := url.Values{}
	q.Set("broadcaster_id", broadcasterID)
	q.Set("moderator_id", modID)
	resp, err := c.helixRequest("PUT", "/helix/moderation/shield_mode?"+q.Encode(), map[string]bool{"is_active": active})
	if err != nil {
		return false, err
	}
	var payload struct {
		Data []struct {
			IsActive bool `json:"is_active"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return false, fmt.Errorf("decode shield mode update: %w", err)
	}
	if len(payload.Data) == 0 {
		return active, nil
	}
	return payload.Data[0].IsActive, nil
}

// StartCommercial triggers a commercial break of the requested length (30..180s).
func (c *Client) StartCommercial(lengthSec int) (CommercialResult, error) {
	broadcasterID, err := c.EnsureUserID()
	if err != nil {
		return CommercialResult{}, err
	}
	resp, err := c.helixRequest("POST", "/helix/channels/commercial", map[string]any{
		"broadcaster_id": broadcasterID,
		"length":         lengthSec,
	})
	if err != nil {
		return CommercialResult{}, err
	}
	var payload struct {
		Data []CommercialResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return CommercialResult{}, fmt.Errorf("decode commercial response: %w", err)
	}
	if len(payload.Data) == 0 {
		return CommercialResult{Length: lengthSec}, nil
	}
	return payload.Data[0], nil
}

// CreateClip creates a Twitch clip for the target broadcaster.
func (c *Client) CreateClip(broadcasterID string, hasDelay bool) (ClipResult, error) {
	q := url.Values{}
	q.Set("broadcaster_id", broadcasterID)
	q.Set("has_delay", strconv.FormatBool(hasDelay))
	resp, err := c.helixRequest("POST", "/helix/clips?"+q.Encode(), nil)
	if err != nil {
		return ClipResult{}, err
	}
	var payload struct {
		Data []ClipResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return ClipResult{}, fmt.Errorf("decode clip response: %w", err)
	}
	if len(payload.Data) == 0 {
		return ClipResult{}, errors.New("empty clip response")
	}
	return payload.Data[0], nil
}

// CreateStreamMarker drops a timestamp marker on the live broadcast.
func (c *Client) CreateStreamMarker(description string) (MarkerResult, error) {
	userID, err := c.EnsureUserID()
	if err != nil {
		return MarkerResult{}, err
	}
	body := map[string]string{
		"user_id": userID,
	}
	if desc := strings.TrimSpace(description); desc != "" {
		if len(desc) > 140 {
			desc = desc[:140]
		}
		body["description"] = desc
	}
	resp, err := c.helixRequest("POST", "/helix/streams/markers", body)
	if err != nil {
		return MarkerResult{}, err
	}
	var payload struct {
		Data []MarkerResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return MarkerResult{}, fmt.Errorf("decode stream marker response: %w", err)
	}
	if len(payload.Data) == 0 {
		return MarkerResult{}, errors.New("empty stream marker response")
	}
	return payload.Data[0], nil
}

// GetStream fetches live stream status and concurrent viewer count.
func (c *Client) GetStream(broadcasterID string) (StreamInfo, error) {
	resp, err := c.helixRequest("GET", "/helix/streams?user_id="+url.QueryEscape(broadcasterID), nil)
	if err != nil {
		return StreamInfo{}, err
	}
	var payload struct {
		Data []struct {
			ViewerCount int    `json:"viewer_count"`
			Title       string `json:"title"`
			GameName    string `json:"game_name"`
			Type        string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return StreamInfo{}, fmt.Errorf("decode stream info: %w", err)
	}
	if len(payload.Data) == 0 {
		return StreamInfo{Live: false}, nil
	}
	s := payload.Data[0]
	return StreamInfo{
		Live:        true,
		ViewerCount: s.ViewerCount,
		Title:       s.Title,
		GameName:    s.GameName,
	}, nil
}

func checkHTTPTransport(resp HTTPResponse) error {
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	if resp.Status == 0 {
		return errors.New("empty HTTP status from host")
	}
	return nil
}

func parseAPIError(resp HTTPResponse) *APIError {
	var parsed struct {
		Error   string `json:"error"`
		Status  int    `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &parsed); err == nil && (parsed.Message != "" || parsed.Error != "") {
		msg := parsed.Message
		if msg == "" {
			msg = parsed.Error
		}
		return &APIError{
			Status:  resp.Status,
			Code:    parsed.Error,
			Message: msg,
		}
	}
	msg := strings.TrimSpace(resp.Body)
	if len(msg) > 120 {
		msg = msg[:120]
	}
	return &APIError{
		Status:  resp.Status,
		Message: msg,
	}
}
