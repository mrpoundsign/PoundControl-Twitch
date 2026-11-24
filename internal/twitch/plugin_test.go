package twitch

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fakeHost struct {
	now            time.Time
	globalSettings map[string]any
	visuals        map[string][]VisualPatch
	timers         map[string]uint32
	logs           []string
	requests       []HTTPRequest
	handler        func(req HTTPRequest) HTTPResponse
}

func newFakeHost() *fakeHost {
	return &fakeHost{
		now:            time.Unix(1_700_000_000, 0),
		globalSettings: make(map[string]any),
		visuals:        make(map[string][]VisualPatch),
		timers:         make(map[string]uint32),
	}
}

func (f *fakeHost) UpdateVisual(ctxID string, patch VisualPatch) {
	f.visuals[ctxID] = append(f.visuals[ctxID], patch)
}

func (f *fakeHost) SaveSettings(_ string, _ map[string]any) error {
	return nil
}

func (f *fakeHost) GetGlobalSettings() map[string]any {
	raw, _ := json.Marshal(f.globalSettings)
	var cp map[string]any
	_ = json.Unmarshal(raw, &cp)
	if cp == nil {
		cp = make(map[string]any)
	}
	return cp
}

func (f *fakeHost) SaveGlobalSettings(settings map[string]any) error {
	raw, _ := json.Marshal(settings)
	var cp map[string]any
	_ = json.Unmarshal(raw, &cp)
	f.globalSettings = cp
	return nil
}

func (f *fakeHost) Log(msg string) {
	f.logs = append(f.logs, msg)
}

func (f *fakeHost) HTTPRequest(req HTTPRequest) HTTPResponse {
	f.requests = append(f.requests, req)
	if f.handler != nil {
		return f.handler(req)
	}
	return HTTPResponse{Status: 500, Body: `{"message":"unhandled mock request"}`}
}

func (f *fakeHost) SetTimer(timerID, _ string, intervalMs uint32) error {
	if intervalMs == 0 {
		delete(f.timers, timerID)
	} else {
		f.timers[timerID] = intervalMs
	}
	return nil
}

func (f *fakeHost) Now() time.Time {
	return f.now
}

func (f *fakeHost) lastVisual(t *testing.T, ctxID string) VisualPatch {
	t.Helper()
	list := f.visuals[ctxID]
	if len(list) == 0 {
		t.Fatalf("expected visual update for %q, got none", ctxID)
	}
	return list[len(list)-1]
}

func seedSignedInHost(host *fakeHost) {
	host.globalSettings = map[string]any{
		SettingClientID:     "test-client-id",
		SettingAccessToken:  "tok-valid",
		SettingRefreshToken: "ref-valid",
		SettingExpiresAt:    float64(host.now.Unix() + 3600),
		SettingUserID:       "1001",
		SettingUserLogin:    "streamer",
		SettingDisplayName:  "Streamer",
	}
}

func TestBuildManifest(t *testing.T) {
	m := BuildManifest()
	if m.UUID != PluginUUID {
		t.Fatalf("unexpected manifest UUID: %q", m.UUID)
	}
	if len(m.Permissions.AllowedHosts) != 2 {
		t.Fatalf("expected 2 allowed hosts, got %v", m.Permissions.AllowedHosts)
	}
	assertValidSVGDataURI(t, "manifest.icon", m.Icon)
	assertValidSVGDataURI(t, "manifest.categoryIcon", m.CategoryIcon)

	wantGlobalTypes := map[string]string{
		SettingAuthStatus:   "info",
		SettingDeviceCode:   "info",
		SettingVerifyURL:    "link",
		SettingAuthConnect:  "button",
		SettingAuthSignOut:  "button",
		SettingClientID:     "password",
		SettingAccessToken:  "password",
		SettingRefreshToken: "password",
	}
	if len(m.GlobalSettingsSchema) != len(wantGlobalTypes) {
		t.Fatalf("expected %d globalSettingsSchema fields, got %d", len(wantGlobalTypes), len(m.GlobalSettingsSchema))
	}
	for _, f := range m.GlobalSettingsSchema {
		wantType, ok := wantGlobalTypes[f.Key]
		if !ok {
			t.Fatalf("unexpected globalSettingsSchema field %q", f.Key)
		}
		if f.Type != wantType {
			t.Fatalf("global field %q type = %q, want %q", f.Key, f.Type, wantType)
		}
		if f.Key == SettingClientID && f.Default != nil {
			t.Fatalf("expected %q Default to be nil so built-in Client ID stays hidden, got %v", SettingClientID, f.Default)
		}
		delete(wantGlobalTypes, f.Key)
	}
	if len(wantGlobalTypes) != 0 {
		t.Fatalf("missing globalSettingsSchema fields: %v", wantGlobalTypes)
	}

	if len(m.Actions) != 12 {
		t.Fatalf("expected 12 actions, got %d", len(m.Actions))
	}
	seen := make(map[string]struct{}, len(m.Actions))
	for _, a := range m.Actions {
		if a.UUID == "" || a.Name == "" {
			t.Fatalf("invalid action descriptor: %+v", a)
		}
		if a.UUID == PluginUUID+".auth" {
			t.Fatalf("unexpected legacy auth action in manifest")
		}
		if _, dup := seen[a.UUID]; dup {
			t.Fatalf("duplicate action UUID: %q", a.UUID)
		}
		seen[a.UUID] = struct{}{}
		assertValidSVGDataURI(t, a.UUID, a.DefaultImage)
	}
}

func assertValidSVGDataURI(t *testing.T, label, uri string) {
	t.Helper()
	const prefix = "data:image/svg+xml;base64,"
	if !strings.HasPrefix(uri, prefix) {
		t.Fatalf("%s: expected base64 SVG data URI, got %q", label, uri)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, prefix))
	if err != nil {
		t.Fatalf("%s: invalid base64 SVG: %v", label, err)
	}
	svg := string(raw)
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, "</svg>") {
		t.Fatalf("%s: decoded payload is not an SVG: %s", label, svg)
	}
}

func assertButtonState(t *testing.T, gs map[string]any, key, wantLabel string, wantDisabled bool) {
	t.Helper()
	raw, ok := gs[key].(map[string]any)
	if !ok {
		t.Fatalf("expected %s to be map, got %T (%v)", key, gs[key], gs[key])
	}
	if got, _ := raw["label"].(string); got != wantLabel {
		t.Fatalf("%s label = %q, want %q", key, got, wantLabel)
	}
	if got, _ := raw["disabled"].(bool); got != wantDisabled {
		t.Fatalf("%s disabled = %v, want %v", key, got, wantDisabled)
	}
}

func TestDeviceAuthFullFlowAndSignOut(t *testing.T) {
	origClientID := DefaultClientID
	DefaultClientID = ""
	t.Cleanup(func() { DefaultClientID = origClientID })

	host := newFakeHost()
	pollStep := 0
	userCalls := 0
	revoked := false

	host.handler = func(req HTTPRequest) HTTPResponse {
		switch {
		case req.Method == "POST" && req.URL == twitchAuthBase+"/oauth2/device":
			vals, _ := url.ParseQuery(req.Body)
			if vals.Get("client_id") != "cid-123" {
				return HTTPResponse{Status: 400, Body: `{"message":"bad client_id"}`}
			}
			if !strings.Contains(vals.Get("scopes"), "moderator:manage:shield_mode") {
				return HTTPResponse{Status: 400, Body: `{"message":"missing scopes"}`}
			}
			return HTTPResponse{
				Status: 200,
				Body:   `{"device_code":"dev-xyz","expires_in":1800,"interval":5,"user_code":"ABCD-1234","verification_uri":"https://www.twitch.tv/activate?device-code=ABCD-1234"}`,
			}
		case req.Method == "POST" && req.URL == twitchAuthBase+"/oauth2/token":
			pollStep++
			switch pollStep {
			case 1:
				return HTTPResponse{Status: 400, Body: `{"status":400,"message":"authorization_pending"}`}
			case 2:
				return HTTPResponse{Status: 400, Body: `{"status":400,"message":"slow_down"}`}
			default:
				return HTTPResponse{
					Status: 200,
					Body:   `{"access_token":"tok-fresh","refresh_token":"ref-fresh","expires_in":14400,"token_type":"bearer"}`,
				}
			}
		case req.Method == "GET" && req.URL == twitchHelixBase+"/helix/users":
			userCalls++
			return HTTPResponse{
				Status: 200,
				Body:   `{"data":[{"id":"42","login":"mrpoundsign","display_name":"MrPoundSign"}]}`,
			}
		case req.Method == "POST" && req.URL == twitchAuthBase+"/oauth2/revoke":
			revoked = true
			return HTTPResponse{Status: 200}
		default:
			return HTTPResponse{Status: 404, Body: `{"message":"unexpected"}`}
		}
	}

	p := NewPlugin(host)

	// Missing Client ID -> error + helpful status message.
	err := p.HandleConfigAction(ConfigActionEvent{Action: SettingAuthConnect})
	if !errors.Is(err, ErrMissingClientID) {
		t.Fatalf("expected ErrMissingClientID, got %v", err)
	}
	if got := host.globalSettings[SettingAuthStatus]; got != "Configure a Twitch Client ID under Advanced first." {
		t.Fatalf("unexpected missing client ID status: %v", got)
	}

	// Connect with transient Client ID -> starts Device Auth.
	if err := p.HandleConfigAction(ConfigActionEvent{
		Action:   SettingAuthConnect,
		Settings: map[string]any{SettingClientID: "cid-123"},
	}); err != nil {
		t.Fatalf("HandleConfigAction(connect) error = %v", err)
	}
	if host.timers[timerAuth] != 5000 {
		t.Fatalf("expected auth timer 5000ms, got %d", host.timers[timerAuth])
	}
	if host.globalSettings[SettingAuthStatus] != "Waiting for Twitch authorization..." {
		t.Fatalf("unexpected pending status: %v", host.globalSettings[SettingAuthStatus])
	}
	dcMap, ok := host.globalSettings[SettingDeviceCode].(map[string]any)
	if !ok || dcMap["text"] != "ABCD-1234" || dcMap["copyable"] != true {
		t.Fatalf("unexpected deviceCode payload: %#v", host.globalSettings[SettingDeviceCode])
	}
	if host.globalSettings[SettingVerifyURL] != "https://www.twitch.tv/activate?device-code=ABCD-1234" {
		t.Fatalf("unexpected verificationUrl: %v", host.globalSettings[SettingVerifyURL])
	}
	assertButtonState(t, host.globalSettings, SettingAuthConnect, "Cancel Sign-In", false)
	assertButtonState(t, host.globalSettings, SettingAuthSignOut, "Sign Out", true)

	// Poll 1: pending.
	_ = p.OnTimer(TimerEvent{TimerID: timerAuth})
	if host.timers[timerAuth] != 5000 {
		t.Fatalf("expected timer to stay 5000ms on pending, got %d", host.timers[timerAuth])
	}

	// Poll 2: slow_down -> increases interval to 10000ms.
	_ = p.OnTimer(TimerEvent{TimerID: timerAuth})
	if host.timers[timerAuth] != 10000 {
		t.Fatalf("expected timer backoff to 10000ms, got %d", host.timers[timerAuth])
	}

	// Poll 3: authorized!
	_ = p.OnTimer(TimerEvent{TimerID: timerAuth})
	if _, stillArmed := host.timers[timerAuth]; stillArmed {
		t.Fatalf("expected auth timer to be cleared after success")
	}
	if host.globalSettings[SettingAccessToken] != "tok-fresh" || host.globalSettings[SettingRefreshToken] != "ref-fresh" {
		t.Fatalf("expected fresh tokens persisted, got %#v", host.globalSettings)
	}
	if host.globalSettings[SettingUserID] != "42" || host.globalSettings[SettingUserLogin] != "mrpoundsign" {
		t.Fatalf("expected user info persisted, got %#v", host.globalSettings)
	}
	if host.globalSettings[SettingDeviceCode] != "" || host.globalSettings[SettingVerifyURL] != "" {
		t.Fatalf("expected pending deviceCode/verificationUrl cleared, got %#v", host.globalSettings)
	}
	if host.globalSettings[SettingAuthStatus] != "Connected as MrPoundSign (@mrpoundsign)" {
		t.Fatalf("unexpected connected status: %v", host.globalSettings[SettingAuthStatus])
	}
	assertButtonState(t, host.globalSettings, SettingAuthConnect, "Verify Connection", false)
	assertButtonState(t, host.globalSettings, SettingAuthSignOut, "Sign Out", false)

	// Clicking authConnect while signed in verifies connection.
	if err := p.HandleConfigAction(ConfigActionEvent{Action: SettingAuthConnect}); err != nil {
		t.Fatalf("verify connection error = %v", err)
	}
	if userCalls != 2 {
		t.Fatalf("expected 2 /helix/users calls (initial + verify), got %d", userCalls)
	}

	// Clicking authSignOut revokes token, clears OS keyring secrets (""), and preserves ClientID.
	if err := p.HandleConfigAction(ConfigActionEvent{Action: SettingAuthSignOut}); err != nil {
		t.Fatalf("sign out error = %v", err)
	}
	if !revoked {
		t.Fatalf("expected token revocation call on signout")
	}
	if p.client.Auth().IsSignedIn() {
		t.Fatalf("expected signed out state")
	}
	if p.client.EffectiveClientID() != "cid-123" || host.globalSettings[SettingClientID] != "cid-123" {
		t.Fatalf("expected ClientID preserved after signout, got client=%q gs=%v", p.client.EffectiveClientID(), host.globalSettings[SettingClientID])
	}
	if tok, has := host.globalSettings[SettingAccessToken]; !has || tok != "" {
		t.Fatalf("expected explicit empty accessToken to clear OS keyring, got (%v, %v)", tok, has)
	}
	if ref, has := host.globalSettings[SettingRefreshToken]; !has || ref != "" {
		t.Fatalf("expected explicit empty refreshToken to clear OS keyring, got (%v, %v)", ref, has)
	}
	if host.globalSettings[SettingUserID] != "" || host.globalSettings[SettingUserLogin] != "" {
		t.Fatalf("expected user info cleared on signout, got %#v", host.globalSettings)
	}
	if host.globalSettings[SettingAuthStatus] != "Signed out" {
		t.Fatalf("unexpected signout status: %v", host.globalSettings[SettingAuthStatus])
	}
	assertButtonState(t, host.globalSettings, SettingAuthConnect, "Sign In with Twitch", false)
	assertButtonState(t, host.globalSettings, SettingAuthSignOut, "Sign Out", true)
}

func TestDeviceAuthCancel(t *testing.T) {
	host := newFakeHost()
	host.handler = func(req HTTPRequest) HTTPResponse {
		if req.Method == "POST" && req.URL == twitchAuthBase+"/oauth2/device" {
			return HTTPResponse{
				Status: 200,
				Body:   `{"device_code":"dev-cancel","expires_in":1800,"interval":5,"user_code":"CANC-5678","verification_uri":"https://www.twitch.tv/activate"}`,
			}
		}
		return HTTPResponse{Status: 404}
	}

	p := NewPlugin(host)
	if err := p.HandleConfigAction(ConfigActionEvent{
		Action:   SettingAuthConnect,
		Settings: map[string]any{SettingClientID: "cid-cancel"},
	}); err != nil {
		t.Fatalf("start device auth error = %v", err)
	}
	if host.timers[timerAuth] != 5000 {
		t.Fatalf("expected auth timer armed")
	}

	// Second click cancels pending Device Auth cleanly.
	if err := p.HandleConfigAction(ConfigActionEvent{Action: SettingAuthConnect}); err != nil {
		t.Fatalf("cancel device auth error = %v", err)
	}
	if _, stillArmed := host.timers[timerAuth]; stillArmed {
		t.Fatalf("expected auth timer stopped on cancel")
	}
	if host.globalSettings[SettingDeviceCode] != "" || host.globalSettings[SettingVerifyURL] != "" {
		t.Fatalf("expected deviceCode and verificationUrl cleared on cancel, got %#v", host.globalSettings)
	}
	if host.globalSettings[SettingAuthStatus] != "Sign-in canceled" {
		t.Fatalf("unexpected cancel status: %v", host.globalSettings[SettingAuthStatus])
	}
	assertButtonState(t, host.globalSettings, SettingAuthConnect, "Sign In with Twitch", false)
	assertButtonState(t, host.globalSettings, SettingAuthSignOut, "Sign Out", true)
}

func TestHandleGlobalSettingsAndLegacyMigration(t *testing.T) {
	t.Run("legacy_migration", func(t *testing.T) {
		host := newFakeHost()
		host.globalSettings = map[string]any{
			legacyAuthKey: map[string]any{
				"clientId":     "legacy-cid",
				"accessToken":  "legacy-tok",
				"refreshToken": "legacy-ref",
				"expiresAt":    float64(host.now.Unix() + 3600),
				"userId":       "777",
				"userLogin":    "legacyuser",
				"displayName":  "LegacyUser",
			},
		}
		p := NewPlugin(host)
		_ = p.HandleEvent(ActionEvent{
			Type:       "willAppear",
			Context:    "v-legacy",
			ActionUUID: ActionViewerCount,
		})
		if _, hasLegacy := host.globalSettings[legacyAuthKey]; hasLegacy {
			t.Fatalf("expected legacy auth key removed after migration")
		}
		if host.globalSettings[SettingAccessToken] != "legacy-tok" || host.globalSettings[SettingUserLogin] != "legacyuser" {
			t.Fatalf("expected migrated flat keys, got %#v", host.globalSettings)
		}
		if host.globalSettings[SettingAuthStatus] != "Connected as LegacyUser (@legacyuser)" {
			t.Fatalf("unexpected migrated authStatus: %v", host.globalSettings[SettingAuthStatus])
		}
	})

	t.Run("edit_client_id_preserves_session_and_reset_clears", func(t *testing.T) {
		host := newFakeHost()
		seedSignedInHost(host)
		host.handler = func(req HTTPRequest) HTTPResponse {
			if req.Method == "GET" && strings.HasPrefix(req.URL, twitchHelixBase+"/helix/streams") {
				return HTTPResponse{Status: 200, Body: `{"data":[]}`}
			}
			return HTTPResponse{Status: 404}
		}

		p := NewPlugin(host)
		_ = p.HandleEvent(ActionEvent{
			Type:       "willAppear",
			Context:    "v-1",
			ActionUUID: ActionViewerCount,
		})
		if host.timers[timerPoll] == 0 {
			t.Fatalf("expected poll timer armed while signed in")
		}

		// Save only changes clientId -> keeps active session & user identity intact.
		if err := p.HandleGlobalSettings(map[string]any{
			SettingClientID:     "updated-cid",
			SettingAccessToken:  "tok-valid",
			SettingRefreshToken: "ref-valid",
		}); err != nil {
			t.Fatalf("HandleGlobalSettings(clientId edit) error = %v", err)
		}
		if p.client.Auth().UserID != "1001" || p.client.Auth().AccessToken != "tok-valid" {
			t.Fatalf("expected session preserved, got %+v", p.client.Auth())
		}
		if host.globalSettings[SettingClientID] != "updated-cid" {
			t.Fatalf("expected updated clientId persisted, got %v", host.globalSettings[SettingClientID])
		}

		// Clearing accessToken (e.g. Reset Defaults) clears auth, stops poll timer, updates ViewerCount tile.
		if err := p.HandleGlobalSettings(map[string]any{
			SettingClientID:     "updated-cid",
			SettingAccessToken:  "",
			SettingRefreshToken: "",
		}); err != nil {
			t.Fatalf("HandleGlobalSettings(reset) error = %v", err)
		}
		if p.client.Auth().IsSignedIn() {
			t.Fatalf("expected signed out after clearing accessToken")
		}
		if _, armed := host.timers[timerPoll]; armed {
			t.Fatalf("expected poll timer stopped after clearing accessToken")
		}
		if v := host.lastVisual(t, "v-1"); v.Subtitle == nil || *v.Subtitle != "Sign In" {
			t.Fatalf("expected ViewerCount tile to revert to Sign In, got %+v", v)
		}
		if host.globalSettings[SettingAuthStatus] != "Not signed in" {
			t.Fatalf("unexpected reset authStatus: %v", host.globalSettings[SettingAuthStatus])
		}
	})
}

func TestAutoRefreshOn401(t *testing.T) {
	host := newFakeHost()
	seedSignedInHost(host)

	cleared := false
	refreshed := false
	host.handler = func(req HTTPRequest) HTTPResponse {
		switch {
		case req.Method == "DELETE" && strings.HasPrefix(req.URL, twitchHelixBase+"/helix/moderation/chat"):
			if req.Headers["Authorization"] == "Bearer tok-valid" {
				return HTTPResponse{Status: 401, Body: `{"status":401,"message":"Invalid OAuth token"}`}
			}
			if req.Headers["Authorization"] == "Bearer tok-refreshed" {
				cleared = true
				return HTTPResponse{Status: 204}
			}
			return HTTPResponse{Status: 401}
		case req.Method == "POST" && req.URL == twitchAuthBase+"/oauth2/token":
			refreshed = true
			return HTTPResponse{
				Status: 200,
				Body:   `{"access_token":"tok-refreshed","refresh_token":"ref-2","expires_in":14400}`,
			}
		default:
			return HTTPResponse{Status: 404}
		}
	}

	p := NewPlugin(host)
	_ = p.HandleEvent(ActionEvent{
		Type:       "keyDown",
		Context:    "clear-1",
		ActionUUID: ActionClearChat,
	})

	if !refreshed || !cleared {
		t.Fatalf("expected token refresh and retry to succeed (refreshed=%v cleared=%v)", refreshed, cleared)
	}
	v := host.lastVisual(t, "clear-1")
	if v.Flash != "ok" || v.Subtitle == nil || *v.Subtitle != "Cleared" {
		t.Fatalf("unexpected clear chat visual: %+v", v)
	}
}

func TestChannelInfoAndCategoryLookup(t *testing.T) {
	host := newFakeHost()
	seedSignedInHost(host)

	var patched ChannelInfoPatch
	host.handler = func(req HTTPRequest) HTTPResponse {
		switch {
		case req.Method == "GET" && strings.HasPrefix(req.URL, twitchHelixBase+"/helix/games?name="):
			return HTTPResponse{Status: 200, Body: `{"data":[]}`}
		case req.Method == "GET" && strings.HasPrefix(req.URL, twitchHelixBase+"/helix/search/categories?query="):
			return HTTPResponse{Status: 200, Body: `{"data":[{"id":"1469308723","name":"Software and Game Development"}]}`}
		case req.Method == "PATCH" && req.URL == twitchHelixBase+"/helix/channels?broadcaster_id=1001":
			_ = json.Unmarshal([]byte(req.Body), &patched)
			return HTTPResponse{Status: 204}
		default:
			return HTTPResponse{Status: 404}
		}
	}

	p := NewPlugin(host)
	_ = p.HandleEvent(ActionEvent{
		Type:       "keyDown",
		Context:    "info-1",
		ActionUUID: ActionChannelInfo,
		Settings: map[string]any{
			"title":    "Building PoundControl!",
			"category": "Software Dev",
			"language": "en",
			"tags":     "Go, OpenSource, StreamDeck",
		},
	})

	if patched.Title != "Building PoundControl!" || patched.GameID != "1469308723" || patched.BroadcasterLanguage != "en" {
		t.Fatalf("unexpected channel patch: %+v", patched)
	}
	if len(patched.Tags) != 3 {
		t.Fatalf("expected 3 tags, got %v", patched.Tags)
	}
	if v := host.lastVisual(t, "info-1"); v.Flash != "ok" {
		t.Fatalf("expected ok flash, got %+v", v)
	}
}

func TestChatModesAndShieldModeToggles(t *testing.T) {
	host := newFakeHost()
	seedSignedInHost(host)

	curChat := ChatSettings{EmoteMode: false, FollowerMode: false, SubscriberMode: false, SlowMode: false}
	curShield := false

	host.handler = func(req HTTPRequest) HTTPResponse {
		switch {
		case req.Method == "GET" && strings.HasPrefix(req.URL, twitchHelixBase+"/helix/chat/settings"):
			raw, _ := json.Marshal(map[string]any{"data": []ChatSettings{curChat}})
			return HTTPResponse{Status: 200, Body: string(raw)}
		case req.Method == "PATCH" && strings.HasPrefix(req.URL, twitchHelixBase+"/helix/chat/settings"):
			var patch ChatSettingsPatch
			_ = json.Unmarshal([]byte(req.Body), &patch)
			if patch.EmoteMode != nil {
				curChat.EmoteMode = *patch.EmoteMode
			}
			if patch.FollowerMode != nil {
				curChat.FollowerMode = *patch.FollowerMode
				curChat.FollowerModeDuration = patch.FollowerModeDuration
			}
			if patch.SubscriberMode != nil {
				curChat.SubscriberMode = *patch.SubscriberMode
			}
			if patch.SlowMode != nil {
				curChat.SlowMode = *patch.SlowMode
				curChat.SlowModeWaitTime = patch.SlowModeWaitTime
			}
			raw, _ := json.Marshal(map[string]any{"data": []ChatSettings{curChat}})
			return HTTPResponse{Status: 200, Body: string(raw)}
		case req.Method == "GET" && strings.HasPrefix(req.URL, twitchHelixBase+"/helix/moderation/shield_mode"):
			raw, _ := json.Marshal(map[string]any{"data": []map[string]bool{{"is_active": curShield}}})
			return HTTPResponse{Status: 200, Body: string(raw)}
		case req.Method == "PUT" && strings.HasPrefix(req.URL, twitchHelixBase+"/helix/moderation/shield_mode"):
			var body struct {
				IsActive bool `json:"is_active"`
			}
			_ = json.Unmarshal([]byte(req.Body), &body)
			curShield = body.IsActive
			raw, _ := json.Marshal(map[string]any{"data": []map[string]bool{{"is_active": curShield}}})
			return HTTPResponse{Status: 200, Body: string(raw)}
		default:
			return HTTPResponse{Status: 404}
		}
	}

	p := NewPlugin(host)
	for _, ev := range []ActionEvent{
		{Type: "willAppear", Context: "emote-1", ActionUUID: ActionEmoteChat},
		{Type: "willAppear", Context: "follow-1", ActionUUID: ActionFollowersChat, Settings: map[string]any{"durationMinutes": "30"}},
		{Type: "willAppear", Context: "sub-1", ActionUUID: ActionSubChat},
		{Type: "willAppear", Context: "slow-1", ActionUUID: ActionSlowChat, Settings: map[string]any{"waitSeconds": "15"}},
		{Type: "willAppear", Context: "shield-1", ActionUUID: ActionShieldMode},
	} {
		_ = p.HandleEvent(ev)
	}

	// Poll timer should be armed automatically.
	if host.timers[timerPoll] == 0 {
		t.Fatalf("expected poll timer to be armed when live status buttons appear")
	}

	// Toggle each mode ON.
	_ = p.HandleEvent(ActionEvent{Type: "keyDown", Context: "emote-1", ActionUUID: ActionEmoteChat})
	_ = p.HandleEvent(ActionEvent{Type: "keyDown", Context: "follow-1", ActionUUID: ActionFollowersChat, Settings: map[string]any{"durationMinutes": "30"}})
	_ = p.HandleEvent(ActionEvent{Type: "keyDown", Context: "sub-1", ActionUUID: ActionSubChat})
	_ = p.HandleEvent(ActionEvent{Type: "keyDown", Context: "slow-1", ActionUUID: ActionSlowChat, Settings: map[string]any{"waitSeconds": "15"}})
	_ = p.HandleEvent(ActionEvent{Type: "keyDown", Context: "shield-1", ActionUUID: ActionShieldMode})

	if !curChat.EmoteMode || !curChat.FollowerMode || !curChat.SubscriberMode || !curChat.SlowMode || !curShield {
		t.Fatalf("expected all modes active, got chat=%+v shield=%v", curChat, curShield)
	}
	if v := host.lastVisual(t, "follow-1"); v.Subtitle == nil || *v.Subtitle != "30m" || v.BackgroundColor == nil || *v.BackgroundColor != colorActiveBg {
		t.Fatalf("expected followers subtitle 30m with active background, got %+v", v)
	}
	if v := host.lastVisual(t, "slow-1"); v.Subtitle == nil || *v.Subtitle != "15s" || v.BackgroundColor == nil || *v.BackgroundColor != colorActiveBg {
		t.Fatalf("expected slow subtitle 15s with active background, got %+v", v)
	}
	if v := host.lastVisual(t, "shield-1"); v.Subtitle == nil || *v.Subtitle != "ACTIVE" || *v.Color != colorShieldOn || v.BackgroundColor == nil || *v.BackgroundColor != colorShieldBg {
		t.Fatalf("expected shield ACTIVE red visual with shield background, got %+v", v)
	}

	// Toggle shield and emote mode back OFF -> clears background override.
	_ = p.HandleEvent(ActionEvent{Type: "keyDown", Context: "emote-1", ActionUUID: ActionEmoteChat})
	_ = p.HandleEvent(ActionEvent{Type: "keyDown", Context: "shield-1", ActionUUID: ActionShieldMode})
	if v := host.lastVisual(t, "emote-1"); v.BackgroundColor == nil || *v.BackgroundColor != "" {
		t.Fatalf("expected inactive emote mode to clear BackgroundColor, got %+v", v)
	}
	if v := host.lastVisual(t, "shield-1"); v.BackgroundColor == nil || *v.BackgroundColor != "" {
		t.Fatalf("expected inactive shield mode to clear BackgroundColor, got %+v", v)
	}

	// Disappear all slots -> poll timer stops.
	for _, id := range []string{"emote-1", "follow-1", "sub-1", "slow-1", "shield-1"} {
		_ = p.HandleEvent(ActionEvent{Type: "willDisappear", Context: id})
	}
	if _, stillArmed := host.timers[timerPoll]; stillArmed {
		t.Fatalf("expected poll timer to stop after all live buttons disappear")
	}
}

func TestBroadcastActionsAdClipMarkerChatAndViewers(t *testing.T) {
	host := newFakeHost()
	seedSignedInHost(host)

	var chatMessages []string
	host.handler = func(req HTTPRequest) HTTPResponse {
		switch {
		case req.Method == "POST" && req.URL == twitchHelixBase+"/helix/channels/commercial":
			return HTTPResponse{Status: 200, Body: `{"data":[{"length":90,"message":"","retry_after":480}]}`}
		case req.Method == "POST" && strings.HasPrefix(req.URL, twitchHelixBase+"/helix/clips"):
			return HTTPResponse{Status: 202, Body: `{"data":[{"id":"EpicClip42","edit_url":"https://clips.twitch.tv/EpicClip42/edit"}]}`}
		case req.Method == "POST" && req.URL == twitchHelixBase+"/helix/chat/messages":
			var body struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal([]byte(req.Body), &body)
			chatMessages = append(chatMessages, body.Message)
			return HTTPResponse{Status: 200, Body: `{"data":[{"message_id":"m1","is_sent":true}]}`}
		case req.Method == "POST" && req.URL == twitchHelixBase+"/helix/streams/markers":
			return HTTPResponse{Status: 200, Body: `{"data":[{"id":"mk1","position_seconds":3725,"description":"Boss kill"}]}`}
		case req.Method == "GET" && req.URL == twitchHelixBase+"/helix/streams?user_id=1001":
			return HTTPResponse{Status: 200, Body: `{"data":[{"viewer_count":1250,"title":"Live!","game_name":"Just Chatting","type":"live"}]}`}
		default:
			return HTTPResponse{Status: 404}
		}
	}

	p := NewPlugin(host)

	// Play Ad.
	_ = p.HandleEvent(ActionEvent{
		Type:       "keyDown",
		Context:    "ad-1",
		ActionUUID: ActionPlayAd,
		Settings:   map[string]any{"length": "90"},
	})
	if v := host.lastVisual(t, "ad-1"); v.Subtitle == nil || *v.Subtitle != "90s Ad Live" {
		t.Fatalf("unexpected ad visual: %+v", v)
	}

	// Create Clip with postToChat enabled.
	_ = p.HandleEvent(ActionEvent{
		Type:       "keyDown",
		Context:    "clip-1",
		ActionUUID: ActionCreateClip,
		Settings:   map[string]any{"postToChat": true},
	})
	if len(chatMessages) != 1 || !strings.Contains(chatMessages[0], "https://clips.twitch.tv/EpicClip42") {
		t.Fatalf("expected clip URL posted to chat, got %v", chatMessages)
	}

	// Direct Chat Message.
	_ = p.HandleEvent(ActionEvent{
		Type:       "keyDown",
		Context:    "chat-1",
		ActionUUID: ActionChatMessage,
		Settings:   map[string]any{"message": "Hello chat!"},
	})
	if len(chatMessages) != 2 || chatMessages[1] != "Hello chat!" {
		t.Fatalf("expected chat message sent, got %v", chatMessages)
	}

	// Stream Marker.
	_ = p.HandleEvent(ActionEvent{
		Type:       "keyDown",
		Context:    "marker-1",
		ActionUUID: ActionStreamMarker,
		Settings:   map[string]any{"description": "Boss kill"},
	})
	if v := host.lastVisual(t, "marker-1"); v.Subtitle == nil || *v.Subtitle != "1:02:05" {
		t.Fatalf("expected marker timestamp 1:02:05, got %+v", v)
	}

	// Viewer Count via poll timer.
	_ = p.HandleEvent(ActionEvent{
		Type:       "willAppear",
		Context:    "viewers-1",
		ActionUUID: ActionViewerCount,
		Settings:   map[string]any{"pollSeconds": 15},
	})
	if host.timers[timerPoll] != 15000 {
		t.Fatalf("expected poll timer 15000ms, got %d", host.timers[timerPoll])
	}
	_ = p.OnTimer(TimerEvent{TimerID: timerPoll})
	if v := host.lastVisual(t, "viewers-1"); v.Subtitle == nil || *v.Subtitle != "1.25K" {
		t.Fatalf("expected viewer count 1.25K, got %+v", v)
	}
}

func TestDefaultClientIDNotPersistedAndCustomClientIDMasked(t *testing.T) {
	if DefaultClientID == "" {
		t.Skip("DefaultClientID is empty")
	}

	host := newFakeHost()
	p := NewPlugin(host)

	// Saving settings without a custom clientId uses DefaultClientID internally and stores "".
	if err := p.HandleGlobalSettings(map[string]any{}); err != nil {
		t.Fatalf("HandleGlobalSettings(empty) error = %v", err)
	}
	if got := host.globalSettings[SettingClientID]; got != "" {
		t.Fatalf("expected %q persisted as empty when using DefaultClientID, got %v", SettingClientID, got)
	}
	if p.client.EffectiveClientID() != DefaultClientID {
		t.Fatalf("EffectiveClientID() = %q, want %q", p.client.EffectiveClientID(), DefaultClientID)
	}

	// Saving a custom clientId persists it for the masked OS keyring entry.
	if err := p.HandleGlobalSettings(map[string]any{
		SettingClientID: "custom-client-id",
	}); err != nil {
		t.Fatalf("HandleGlobalSettings(custom) error = %v", err)
	}
	if got := host.globalSettings[SettingClientID]; got != "custom-client-id" {
		t.Fatalf("expected custom clientId persisted, got %v", got)
	}
	if p.client.EffectiveClientID() != "custom-client-id" {
		t.Fatalf("EffectiveClientID() = %q, want %q", p.client.EffectiveClientID(), "custom-client-id")
	}

	// Clearing the custom clientId reverts to DefaultClientID and stores "".
	if err := p.HandleGlobalSettings(map[string]any{
		SettingClientID: "",
	}); err != nil {
		t.Fatalf("HandleGlobalSettings(clear) error = %v", err)
	}
	if got := host.globalSettings[SettingClientID]; got != "" {
		t.Fatalf("expected cleared clientId persisted as empty, got %v", got)
	}
	if p.client.EffectiveClientID() != DefaultClientID {
		t.Fatalf("EffectiveClientID() = %q, want %q", p.client.EffectiveClientID(), DefaultClientID)
	}
}
