package twitch

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
)

const (
	timerAuth = "twitch-auth"
	timerPoll = "twitch-poll"

	defaultPollSeconds = 30
	minPollSeconds     = 10
	legacyAuthKey      = "auth"
)

type slotState struct {
	ctxID      string
	actionUUID string
	settings   map[string]any
}

type pendingDeviceAuth struct {
	clientID   string
	deviceCode string
	userCode   string
	verifyURI  string
	intervalS  int
	expiresAt  int64
}

// Plugin is the core Twitch Wasm plugin controller.
type Plugin struct {
	mu           sync.Mutex
	host         Host
	client       *Client
	authLoaded   bool
	slots        map[string]*slotState
	pendingAuth  *pendingDeviceAuth
	pollInterval uint32 // currently armed poll timer interval in ms (0 = stopped)

	chatCache   map[string]ChatSettings // broadcasterID -> latest ChatSettings
	shieldCache map[string]bool         // broadcasterID -> latest shield active
	streamCache map[string]StreamInfo   // broadcasterID -> latest StreamInfo
}

// NewPlugin constructs a Twitch Plugin bound to host.
func NewPlugin(host Host) *Plugin {
	p := &Plugin{
		host:        host,
		slots:       make(map[string]*slotState),
		chatCache:   make(map[string]ChatSettings),
		shieldCache: make(map[string]bool),
		streamCache: make(map[string]StreamInfo),
	}
	p.client = NewClient(host, AuthState{ClientID: DefaultClientID}, p.persistAuth)
	return p
}

func (p *Plugin) ensureAuthLoaded() {
	if p.authLoaded {
		return
	}
	p.authLoaded = true
	gs := p.host.GetGlobalSettings()
	if len(gs) == 0 {
		return
	}

	a, hasFlat := authStateFromSettings(gs)
	migratedLegacy := false
	if a.AccessToken == "" && a.RefreshToken == "" {
		if rawAuth, ok := gs[legacyAuthKey]; ok && rawAuth != nil {
			if b, err := json.Marshal(rawAuth); err == nil {
				var legacy AuthState
				if json.Unmarshal(b, &legacy) == nil && (legacy.AccessToken != "" || legacy.ClientID != "" || legacy.UserID != "") {
					if a.ClientID != "" && legacy.ClientID == "" {
						legacy.ClientID = a.ClientID
					}
					a = legacy
					migratedLegacy = true
				}
			}
		}
	}

	hasStalePending := p.pendingAuth == nil && (hasNonEmptySetting(gs[SettingDeviceCode]) || hasNonEmptySetting(gs[SettingVerifyURL]))
	if !hasFlat && !migratedLegacy && !hasStalePending {
		return
	}
	if strings.TrimSpace(a.ClientID) == "" && DefaultClientID != "" {
		a.ClientID = DefaultClientID
	}
	p.client.auth = a
	if migratedLegacy || hasStalePending {
		p.persistConfigState("", false)
	}
}

func authStateFromSettings(gs map[string]any) (AuthState, bool) {
	a := AuthState{
		ClientID:     settingString(gs, SettingClientID),
		AccessToken:  settingString(gs, SettingAccessToken),
		RefreshToken: settingString(gs, SettingRefreshToken),
		UserID:       settingString(gs, SettingUserID),
		UserLogin:    settingString(gs, SettingUserLogin),
		DisplayName:  settingString(gs, SettingDisplayName),
		ExpiresAt:    settingInt64(gs, SettingExpiresAt),
	}
	hasAny := a.ClientID != "" || a.AccessToken != "" || a.RefreshToken != "" ||
		a.UserID != "" || a.UserLogin != "" || a.DisplayName != "" || a.ExpiresAt > 0
	return a, hasAny
}

func hasNonEmptySetting(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case map[string]any:
		return strings.TrimSpace(fmt.Sprint(x["text"])) != "" && x["text"] != nil
	default:
		return strings.TrimSpace(fmt.Sprint(x)) != ""
	}
}

func (p *Plugin) persistAuth(a AuthState) {
	clearSecrets := strings.TrimSpace(a.AccessToken) == "" && strings.TrimSpace(a.RefreshToken) == ""
	p.persistConfigState("", clearSecrets)
}

func (p *Plugin) persistConfigState(statusMsg string, clearSecrets bool) {
	a := p.client.Auth()
	clientID := strings.TrimSpace(a.ClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(DefaultClientID)
	}

	gs := map[string]any{
		SettingClientID:    clientID,
		SettingUserID:      a.UserID,
		SettingUserLogin:   a.UserLogin,
		SettingDisplayName: a.DisplayName,
		SettingExpiresAt:   a.ExpiresAt,
	}

	switch {
	case clearSecrets:
		gs[SettingAccessToken] = ""
		gs[SettingRefreshToken] = ""
	case a.IsSignedIn():
		gs[SettingAccessToken] = a.AccessToken
		gs[SettingRefreshToken] = a.RefreshToken
	default:
		if tok := strings.TrimSpace(a.AccessToken); tok != "" {
			gs[SettingAccessToken] = tok
		}
		if ref := strings.TrimSpace(a.RefreshToken); ref != "" {
			gs[SettingRefreshToken] = ref
		}
	}

	switch {
	case p.pendingAuth != nil:
		status := statusMsg
		if status == "" {
			status = "Waiting for Twitch authorization..."
		}
		gs[SettingAuthStatus] = status
		gs[SettingDeviceCode] = map[string]any{
			"text":     p.pendingAuth.userCode,
			"copyable": true,
		}
		gs[SettingVerifyURL] = p.pendingAuth.verifyURI
		gs[SettingAuthConnect] = map[string]any{"label": "Cancel Sign-In", "disabled": false}
		gs[SettingAuthSignOut] = map[string]any{"label": "Sign Out", "disabled": true}
	case a.IsSignedIn():
		status := statusMsg
		if status == "" {
			status = formatConnectedStatus(a)
		}
		gs[SettingAuthStatus] = status
		gs[SettingDeviceCode] = ""
		gs[SettingVerifyURL] = ""
		gs[SettingAuthConnect] = map[string]any{"label": "Verify Connection", "disabled": false}
		gs[SettingAuthSignOut] = map[string]any{"label": "Sign Out", "disabled": false}
	default:
		status := statusMsg
		if status == "" {
			status = "Not signed in"
		}
		gs[SettingAuthStatus] = status
		gs[SettingDeviceCode] = ""
		gs[SettingVerifyURL] = ""
		gs[SettingAuthConnect] = map[string]any{"label": "Sign In with Twitch", "disabled": false}
		gs[SettingAuthSignOut] = map[string]any{"label": "Sign Out", "disabled": true}
	}

	if err := p.host.SaveGlobalSettings(gs); err != nil {
		p.host.Log("failed saving global settings: " + err.Error())
	}
}

func formatConnectedStatus(a AuthState) string {
	switch {
	case a.DisplayName != "" && a.UserLogin != "":
		return fmt.Sprintf("Connected as %s (@%s)", a.DisplayName, a.UserLogin)
	case a.UserLogin != "":
		return fmt.Sprintf("Connected as @%s", a.UserLogin)
	case a.DisplayName != "":
		return fmt.Sprintf("Connected as %s", a.DisplayName)
	default:
		return "Connected"
	}
}

// HandleGlobalSettings synchronizes plugin-wide settings saved via PoundControl's Settings -> Twitch tab.
func (p *Plugin) HandleGlobalSettings(settings map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ensureAuthLoaded()
	prev := p.client.auth

	clientID := settingString(settings, SettingClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(DefaultClientID)
	}
	accessToken := settingString(settings, SettingAccessToken)
	refreshToken := settingString(settings, SettingRefreshToken)

	switch {
	case accessToken == "":
		wasActive := prev.IsSignedIn() || p.pendingAuth != nil || prev.UserID != ""
		p.stopDeviceAuth()
		p.client.auth = AuthState{ClientID: clientID}
		clear(p.chatCache)
		clear(p.shieldCache)
		clear(p.streamCache)
		p.reconcilePollTimer()
		if wasActive {
			for _, slot := range p.slots {
				p.renderSlotIdle(slot)
			}
		}
		p.persistConfigState("Not signed in", true)

	case accessToken != prev.AccessToken:
		p.stopDeviceAuth()
		p.client.auth = AuthState{
			ClientID:     clientID,
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		}
		clear(p.chatCache)
		clear(p.shieldCache)
		clear(p.streamCache)
		if _, err := p.client.GetCurrentUser(); err != nil {
			p.host.Log("Twitch token updated, but profile lookup failed: " + err.Error())
			p.persistConfigState("Connected (profile lookup failed)", false)
		}
		p.reconcilePollTimer()
		p.pollLiveStatus()

	default:
		p.client.auth.ClientID = clientID
		p.client.auth.RefreshToken = refreshToken
		p.persistConfigState("", false)
	}
	return nil
}

// HandleConfigAction handles interactive FieldButton clicks from PoundControl's Settings -> Twitch tab.
func (p *Plugin) HandleConfigAction(ev ConfigActionEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ensureAuthLoaded()

	if rawCID, hasCID := ev.Settings[SettingClientID]; hasCID {
		if rawCID == nil {
			p.client.auth.ClientID = ""
		} else {
			p.client.auth.ClientID = strings.TrimSpace(fmt.Sprint(rawCID))
		}
	}

	switch ev.Action {
	case SettingAuthConnect:
		return p.handleConfigConnect()
	case SettingAuthSignOut:
		return p.handleConfigSignOut()
	default:
		return fmt.Errorf("unknown config action %q", ev.Action)
	}
}

func (p *Plugin) handleConfigConnect() error {
	if p.pendingAuth != nil {
		p.stopDeviceAuth()
		p.host.Log("Twitch Device Auth canceled by user")
		p.persistConfigState("Sign-in canceled", false)
		return nil
	}

	if p.client.Auth().IsSignedIn() {
		u, err := p.client.GetCurrentUser()
		if err != nil {
			p.host.Log("Twitch connection verification failed: " + err.Error())
			p.persistConfigState("Verification failed: "+err.Error(), false)
			return err
		}
		p.host.Log(fmt.Sprintf("Twitch connected as %s (@%s)", u.DisplayName, u.Login))
		return nil
	}

	clientID := p.client.EffectiveClientID()
	if clientID == "" {
		p.persistConfigState("Configure a Twitch Client ID under Advanced first.", false)
		return ErrMissingClientID
	}
	p.client.auth.ClientID = clientID

	resp, err := p.client.StartDeviceAuth(clientID)
	if err != nil {
		p.host.Log("Twitch Device Auth failed: " + err.Error())
		p.persistConfigState("Sign-in failed: "+err.Error(), false)
		return err
	}

	intervalS := max(resp.Interval, 5)
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 1800
	}
	p.pendingAuth = &pendingDeviceAuth{
		clientID:   clientID,
		deviceCode: resp.DeviceCode,
		userCode:   resp.UserCode,
		verifyURI:  resp.VerificationURI,
		intervalS:  intervalS,
		expiresAt:  p.host.Now().Unix() + int64(expiresIn),
	}

	p.host.Log(fmt.Sprintf("Twitch Device Auth started! Visit %s and enter code: %s", resp.VerificationURI, resp.UserCode))
	_ = p.host.SetTimer(timerAuth, "", uint32(intervalS)*1000)
	p.persistConfigState("Waiting for Twitch authorization...", false)
	return nil
}

func (p *Plugin) handleConfigSignOut() error {
	p.stopDeviceAuth()
	p.client.RevokeToken()
	clear(p.chatCache)
	clear(p.shieldCache)
	clear(p.streamCache)
	p.reconcilePollTimer()
	for _, slot := range p.slots {
		p.renderSlotIdle(slot)
	}
	p.persistConfigState("Signed out", true)
	p.host.Log("Signed out of Twitch")
	return nil
}

// HandleEvent processes a PoundControl ActionEvent.
func (p *Plugin) HandleEvent(ev ActionEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ensureAuthLoaded()

	switch ev.Type {
	case "willAppear", "didReceiveSettings":
		slot := &slotState{
			ctxID:      ev.Context,
			actionUUID: ev.ActionUUID,
			settings:   cloneSettings(ev.Settings),
		}
		p.slots[ev.Context] = slot
		p.renderSlotIdle(slot)
		p.reconcilePollTimer()
	case "willDisappear":
		delete(p.slots, ev.Context)
		p.reconcilePollTimer()
	case "keyDown":
		slot, ok := p.slots[ev.Context]
		if !ok {
			slot = &slotState{
				ctxID:      ev.Context,
				actionUUID: ev.ActionUUID,
				settings:   cloneSettings(ev.Settings),
			}
			p.slots[ev.Context] = slot
		} else if len(ev.Settings) > 0 {
			slot.settings = cloneSettings(ev.Settings)
		}
		p.handleKeyDown(slot)
	}
	return nil
}

// OnTimer handles periodic host timer callbacks (Device Auth polling and live status polling).
func (p *Plugin) OnTimer(tev TimerEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ensureAuthLoaded()

	switch tev.TimerID {
	case timerAuth:
		p.pollDeviceAuth()
	case timerPoll:
		p.pollLiveStatus()
	default:
		_ = p.host.SetTimer(tev.TimerID, tev.Context, 0)
	}
	return nil
}

func (p *Plugin) handleKeyDown(slot *slotState) {
	switch slot.actionUUID {
	case ActionChannelInfo:
		p.handleChannelInfoPress(slot)
	case ActionChatMessage:
		p.handleChatMessagePress(slot)
	case ActionClearChat:
		p.handleClearChatPress(slot)
	case ActionEmoteChat:
		p.handleChatModePress(slot, "emote")
	case ActionFollowersChat:
		p.handleChatModePress(slot, "followers")
	case ActionSubChat:
		p.handleChatModePress(slot, "sub")
	case ActionSlowChat:
		p.handleChatModePress(slot, "slow")
	case ActionShieldMode:
		p.handleShieldModePress(slot)
	case ActionPlayAd:
		p.handlePlayAdPress(slot)
	case ActionCreateClip:
		p.handleCreateClipPress(slot)
	case ActionStreamMarker:
		p.handleStreamMarkerPress(slot)
	case ActionViewerCount:
		p.handleViewerCountPress(slot)
	}
}

func (p *Plugin) pollDeviceAuth() {
	pa := p.pendingAuth
	if pa == nil {
		_ = p.host.SetTimer(timerAuth, "", 0)
		return
	}
	if p.host.Now().Unix() >= pa.expiresAt {
		p.stopDeviceAuth()
		p.host.Log("Twitch Device Auth code expired; click Sign In with Twitch to retry")
		p.persistConfigState("Activation code expired — click Sign In to try again.", false)
		return
	}

	tok, err := p.client.PollDeviceToken(pa.clientID, pa.deviceCode)
	if err != nil {
		switch {
		case errors.Is(err, ErrAuthPending):
			return
		case errors.Is(err, ErrSlowDown):
			pa.intervalS += 5
			_ = p.host.SetTimer(timerAuth, "", uint32(pa.intervalS)*1000)
			return
		case errors.Is(err, ErrDeviceExpired):
			p.stopDeviceAuth()
			p.host.Log("Twitch Device Auth expired or denied")
			p.persistConfigState("Authorization expired or denied.", false)
			return
		default:
			p.host.Log("Twitch Device Auth poll error: " + err.Error())
			return
		}
	}

	p.stopDeviceAuth()
	var expiresAt int64
	if tok.ExpiresIn > 0 {
		expiresAt = p.host.Now().Unix() + int64(tok.ExpiresIn)
	}
	p.client.auth = AuthState{
		ClientID:     pa.clientID,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    expiresAt,
	}

	u, err := p.client.GetCurrentUser()
	if err != nil {
		p.host.Log("Twitch signed in, but failed fetching profile: " + err.Error())
		p.persistConfigState("Connected (profile lookup failed)", false)
	} else {
		p.host.Log(fmt.Sprintf("Twitch signed in as %s (@%s)", u.DisplayName, u.Login))
	}

	p.reconcilePollTimer()
	p.pollLiveStatus()
}

func (p *Plugin) stopDeviceAuth() {
	p.pendingAuth = nil
	_ = p.host.SetTimer(timerAuth, "", 0)
}

func (p *Plugin) handleChannelInfoPress(slot *slotState) {
	title := settingString(slot.settings, "title")
	category := settingString(slot.settings, "category")
	lang := settingString(slot.settings, "language")
	tagsRaw := settingString(slot.settings, "tags")

	var patch ChannelInfoPatch
	patch.Title = title
	patch.BroadcasterLanguage = lang

	if category != "" {
		gameID, err := p.client.ResolveGameID(category)
		if err != nil {
			p.reportSlotError(slot, "Bad Category", err)
			return
		}
		patch.GameID = gameID
	}
	if tagsRaw != "" {
		for part := range strings.SplitSeq(tagsRaw, ",") {
			if t := strings.TrimSpace(part); t != "" {
				patch.Tags = append(patch.Tags, t)
				if len(patch.Tags) == 10 {
					break
				}
			}
		}
	}

	if patch.Title == "" && patch.GameID == "" && patch.BroadcasterLanguage == "" && len(patch.Tags) == 0 {
		p.reportSlotError(slot, "Nothing Set", errors.New("configure at least a title, category, language, or tag"))
		return
	}

	if err := p.client.ModifyChannelInfo(patch); err != nil {
		p.reportSlotError(slot, "Update Failed", err)
		return
	}

	sub := "Updated"
	if category != "" {
		sub = truncate(category, 16)
	}
	p.host.UpdateVisual(slot.ctxID, VisualPatch{
		Subtitle: new(sub),
		Color:    new(colorTwitch),
		Flash:    "ok",
	})
}

func (p *Plugin) handleChatMessagePress(slot *slotState) {
	msg := settingString(slot.settings, "message")
	if msg == "" {
		p.reportSlotError(slot, "No Message", errors.New("chat message is empty"))
		return
	}
	broadcasterID, err := p.client.ResolveBroadcasterID(settingString(slot.settings, "channel"))
	if err != nil {
		p.reportSlotError(slot, "Channel Err", err)
		return
	}
	if err := p.client.SendChatMessage(broadcasterID, msg); err != nil {
		p.reportSlotError(slot, "Send Failed", err)
		return
	}
	p.host.UpdateVisual(slot.ctxID, VisualPatch{
		Subtitle: new("Sent"),
		Color:    new(colorTwitch),
		Flash:    "ok",
	})
}

func (p *Plugin) handleClearChatPress(slot *slotState) {
	broadcasterID, err := p.client.ResolveBroadcasterID(settingString(slot.settings, "channel"))
	if err != nil {
		p.reportSlotError(slot, "Channel Err", err)
		return
	}
	if err := p.client.ClearChat(broadcasterID); err != nil {
		p.reportSlotError(slot, "Clear Failed", err)
		return
	}
	p.host.UpdateVisual(slot.ctxID, VisualPatch{
		Subtitle: new("Cleared"),
		Color:    new(colorTwitch),
		Flash:    "ok",
	})
}

func (p *Plugin) handleChatModePress(slot *slotState, kind string) {
	broadcasterID, err := p.client.ResolveBroadcasterID(settingString(slot.settings, "channel"))
	if err != nil {
		p.reportSlotError(slot, "Channel Err", err)
		return
	}

	mode := strings.ToLower(settingString(slot.settings, "mode"))
	var target bool
	switch mode {
	case "on":
		target = true
	case "off":
		target = false
	default: // toggle
		cur, ok := p.chatCache[broadcasterID]
		if !ok {
			fetched, fetchErr := p.client.GetChatSettings(broadcasterID)
			if fetchErr != nil {
				p.reportSlotError(slot, "Fetch Failed", fetchErr)
				return
			}
			cur = fetched
			p.chatCache[broadcasterID] = cur
		}
		switch kind {
		case "emote":
			target = !cur.EmoteMode
		case "followers":
			target = !cur.FollowerMode
		case "sub":
			target = !cur.SubscriberMode
		case "slow":
			target = !cur.SlowMode
		}
	}

	var patch ChatSettingsPatch
	switch kind {
	case "emote":
		patch.EmoteMode = &target
	case "followers":
		patch.FollowerMode = &target
		if target {
			dur := min(max(settingInt(slot.settings, "durationMinutes", 10), 0), 129600)
			patch.FollowerModeDuration = &dur
		}
	case "sub":
		patch.SubscriberMode = &target
	case "slow":
		patch.SlowMode = &target
		if target {
			wait := min(max(settingInt(slot.settings, "waitSeconds", 30), 3), 120)
			patch.SlowModeWaitTime = &wait
		}
	}

	updated, err := p.client.UpdateChatSettings(broadcasterID, patch)
	if err != nil {
		p.reportSlotError(slot, "Update Failed", err)
		return
	}
	p.chatCache[broadcasterID] = updated
	p.renderChatModeSlotsForChannel(broadcasterID, updated, slot.ctxID)
}

func (p *Plugin) handleShieldModePress(slot *slotState) {
	broadcasterID, err := p.client.ResolveBroadcasterID(settingString(slot.settings, "channel"))
	if err != nil {
		p.reportSlotError(slot, "Channel Err", err)
		return
	}

	mode := strings.ToLower(settingString(slot.settings, "mode"))
	var target bool
	switch mode {
	case "on":
		target = true
	case "off":
		target = false
	default: // toggle
		cur, ok := p.shieldCache[broadcasterID]
		if !ok {
			fetched, fetchErr := p.client.GetShieldMode(broadcasterID)
			if fetchErr != nil {
				p.reportSlotError(slot, "Fetch Failed", fetchErr)
				return
			}
			cur = fetched
			p.shieldCache[broadcasterID] = cur
		}
		target = !cur
	}

	active, err := p.client.UpdateShieldMode(broadcasterID, target)
	if err != nil {
		p.reportSlotError(slot, "Shield Failed", err)
		return
	}
	p.shieldCache[broadcasterID] = active
	p.renderShieldSlotsForChannel(broadcasterID, active, slot.ctxID)
}

func (p *Plugin) handlePlayAdPress(slot *slotState) {
	length := settingInt(slot.settings, "length", 60)
	switch length {
	case 30, 60, 90, 120, 150, 180:
	default:
		length = 60
	}

	res, err := p.client.StartCommercial(length)
	if err != nil {
		p.reportSlotError(slot, "Ad Failed", err)
		return
	}
	if res.Length > 0 {
		length = res.Length
	}
	p.host.Log(fmt.Sprintf("Started %ds Twitch commercial (retry after %ds)", length, res.RetryAfter))
	p.host.UpdateVisual(slot.ctxID, VisualPatch{
		Subtitle: new(fmt.Sprintf("%ds Ad Live", length)),
		Color:    new(colorActive),
		Flash:    "ok",
	})
}

func (p *Plugin) handleCreateClipPress(slot *slotState) {
	broadcasterID, err := p.client.ResolveBroadcasterID(settingString(slot.settings, "channel"))
	if err != nil {
		p.reportSlotError(slot, "Channel Err", err)
		return
	}
	hasDelay := settingBool(slot.settings, "hasDelay")
	clip, err := p.client.CreateClip(broadcasterID, hasDelay)
	if err != nil {
		p.reportSlotError(slot, "Clip Failed", err)
		return
	}

	clipURL := "https://clips.twitch.tv/" + clip.ID
	p.host.Log(fmt.Sprintf("Twitch clip created: %s (edit: %s)", clipURL, clip.EditURL))

	if settingBool(slot.settings, "postToChat") {
		if chatErr := p.client.SendChatMessage(broadcasterID, "Clip created: "+clipURL); chatErr != nil {
			p.host.Log("Failed posting clip URL to chat: " + chatErr.Error())
		}
	}

	p.host.UpdateVisual(slot.ctxID, VisualPatch{
		Subtitle: new("Clipped!"),
		Color:    new(colorActive),
		Flash:    "ok",
	})
}

func (p *Plugin) handleStreamMarkerPress(slot *slotState) {
	desc := settingString(slot.settings, "description")
	res, err := p.client.CreateStreamMarker(desc)
	if err != nil {
		p.reportSlotError(slot, "Marker Failed", err)
		return
	}
	p.host.Log(fmt.Sprintf("Twitch stream marker created at %s (id=%s)", formatTimestamp(res.PositionSeconds), res.ID))
	p.host.UpdateVisual(slot.ctxID, VisualPatch{
		Subtitle: new(formatTimestamp(res.PositionSeconds)),
		Color:    new(colorActive),
		Flash:    "ok",
	})
}

func (p *Plugin) handleViewerCountPress(slot *slotState) {
	broadcasterID, err := p.client.ResolveBroadcasterID(settingString(slot.settings, "channel"))
	if err != nil {
		p.reportSlotError(slot, "Channel Err", err)
		return
	}
	info, err := p.client.GetStream(broadcasterID)
	if err != nil {
		p.reportSlotError(slot, "Fetch Failed", err)
		return
	}
	p.streamCache[broadcasterID] = info
	p.renderViewerSlot(slot, info, "ok")
}

func (p *Plugin) pollLiveStatus() {
	if !p.client.Auth().IsSignedIn() {
		p.reconcilePollTimer()
		return
	}

	streamChannels := make(map[string]struct{})
	chatChannels := make(map[string]struct{})
	shieldChannels := make(map[string]struct{})

	for _, slot := range p.slots {
		ch := strings.ToLower(strings.TrimPrefix(settingString(slot.settings, "channel"), "@"))
		switch slot.actionUUID {
		case ActionViewerCount:
			streamChannels[ch] = struct{}{}
		case ActionEmoteChat, ActionFollowersChat, ActionSubChat, ActionSlowChat:
			chatChannels[ch] = struct{}{}
		case ActionShieldMode:
			shieldChannels[ch] = struct{}{}
		}
	}

	for ch := range streamChannels {
		bid, err := p.client.ResolveBroadcasterID(ch)
		if err != nil {
			continue
		}
		info, err := p.client.GetStream(bid)
		if err != nil {
			continue
		}
		p.streamCache[bid] = info
		for _, slot := range p.slots {
			if slot.actionUUID == ActionViewerCount && p.slotMatchesBroadcaster(slot, bid) {
				p.renderViewerSlot(slot, info, "")
			}
		}
	}

	for ch := range chatChannels {
		bid, err := p.client.ResolveBroadcasterID(ch)
		if err != nil {
			continue
		}
		cs, err := p.client.GetChatSettings(bid)
		if err != nil {
			continue
		}
		p.chatCache[bid] = cs
		p.renderChatModeSlotsForChannel(bid, cs, "")
	}

	for ch := range shieldChannels {
		bid, err := p.client.ResolveBroadcasterID(ch)
		if err != nil {
			continue
		}
		active, err := p.client.GetShieldMode(bid)
		if err != nil {
			continue
		}
		p.shieldCache[bid] = active
		p.renderShieldSlotsForChannel(bid, active, "")
	}
}

func (p *Plugin) reconcilePollTimer() {
	if !p.client.Auth().IsSignedIn() {
		if p.pollInterval != 0 {
			p.pollInterval = 0
			_ = p.host.SetTimer(timerPoll, "", 0)
		}
		return
	}

	needPoll := false
	minSec := defaultPollSeconds
	for _, slot := range p.slots {
		switch slot.actionUUID {
		case ActionViewerCount:
			needPoll = true
			sec := max(settingInt(slot.settings, "pollSeconds", defaultPollSeconds), minPollSeconds)
			if sec < minSec {
				minSec = sec
			}
		case ActionEmoteChat, ActionFollowersChat, ActionSubChat, ActionSlowChat, ActionShieldMode:
			needPoll = true
		}
	}

	if !needPoll {
		if p.pollInterval != 0 {
			p.pollInterval = 0
			_ = p.host.SetTimer(timerPoll, "", 0)
		}
		return
	}

	wantMs := uint32(minSec) * 1000
	if p.pollInterval != wantMs {
		p.pollInterval = wantMs
		_ = p.host.SetTimer(timerPoll, "", wantMs)
	}
}

func (p *Plugin) renderSlotIdle(slot *slotState) {
	switch slot.actionUUID {
	case ActionViewerCount:
		if !p.client.Auth().IsSignedIn() {
			p.host.UpdateVisual(slot.ctxID, VisualPatch{
				Subtitle: new("Sign In"),
				Color:    new(colorInactive),
			})
			return
		}
		if bid, ok := p.cachedBroadcasterID(slot); ok {
			if info, has := p.streamCache[bid]; has {
				p.renderViewerSlot(slot, info, "")
				return
			}
		}
		p.host.UpdateVisual(slot.ctxID, VisualPatch{
			Subtitle: new("..."),
			Color:    new(colorTwitch),
		})
	case ActionEmoteChat, ActionFollowersChat, ActionSubChat, ActionSlowChat:
		if bid, ok := p.cachedBroadcasterID(slot); ok {
			if cs, has := p.chatCache[bid]; has {
				p.renderChatModeSlot(slot, cs, "")
				return
			}
		}
		p.host.UpdateVisual(slot.ctxID, VisualPatch{
			Subtitle: new("Off"),
			Color:    new(colorInactive),
			State:    new(0),
		})
	case ActionShieldMode:
		if bid, ok := p.cachedBroadcasterID(slot); ok {
			if active, has := p.shieldCache[bid]; has {
				p.renderShieldSlot(slot, active, "")
				return
			}
		}
		p.host.UpdateVisual(slot.ctxID, VisualPatch{
			Subtitle: new("Off"),
			Color:    new(colorInactive),
			State:    new(0),
		})
	}
}

func (p *Plugin) renderChatModeSlotsForChannel(broadcasterID string, cs ChatSettings, flashCtxID string) {
	for _, slot := range p.slots {
		switch slot.actionUUID {
		case ActionEmoteChat, ActionFollowersChat, ActionSubChat, ActionSlowChat:
			if p.slotMatchesBroadcaster(slot, broadcasterID) {
				flash := ""
				if slot.ctxID == flashCtxID {
					flash = "ok"
				}
				p.renderChatModeSlot(slot, cs, flash)
			}
		}
	}
}

func (p *Plugin) renderChatModeSlot(slot *slotState, cs ChatSettings, flash string) {
	var (
		active bool
		sub    = "Off"
	)
	switch slot.actionUUID {
	case ActionEmoteChat:
		active = cs.EmoteMode
		if active {
			sub = "On"
		}
	case ActionFollowersChat:
		active = cs.FollowerMode
		if active {
			if cs.FollowerModeDuration != nil && *cs.FollowerModeDuration > 0 {
				sub = formatDurationMinutes(*cs.FollowerModeDuration)
			} else {
				sub = "On"
			}
		}
	case ActionSubChat:
		active = cs.SubscriberMode
		if active {
			sub = "On"
		}
	case ActionSlowChat:
		active = cs.SlowMode
		if active {
			if cs.SlowModeWaitTime != nil && *cs.SlowModeWaitTime > 0 {
				sub = fmt.Sprintf("%ds", *cs.SlowModeWaitTime)
			} else {
				sub = "On"
			}
		}
	}

	col := colorInactive
	st := 0
	if active {
		col = colorActive
		st = 1
	}
	p.host.UpdateVisual(slot.ctxID, VisualPatch{
		Subtitle: new(sub),
		Color:    new(col),
		State:    new(st),
		Flash:    flash,
	})
}

func (p *Plugin) renderShieldSlotsForChannel(broadcasterID string, active bool, flashCtxID string) {
	for _, slot := range p.slots {
		if slot.actionUUID == ActionShieldMode && p.slotMatchesBroadcaster(slot, broadcasterID) {
			flash := ""
			if slot.ctxID == flashCtxID {
				flash = "ok"
			}
			p.renderShieldSlot(slot, active, flash)
		}
	}
}

func (p *Plugin) renderShieldSlot(slot *slotState, active bool, flash string) {
	sub := "Off"
	col := colorInactive
	st := 0
	if active {
		sub = "ACTIVE"
		col = colorShieldOn
		st = 1
	}
	p.host.UpdateVisual(slot.ctxID, VisualPatch{
		Subtitle: new(sub),
		Color:    new(col),
		State:    new(st),
		Flash:    flash,
	})
}

func (p *Plugin) renderViewerSlot(slot *slotState, info StreamInfo, flash string) {
	if !info.Live {
		p.host.UpdateVisual(slot.ctxID, VisualPatch{
			Subtitle: new("Offline"),
			Color:    new(colorInactive),
			State:    new(0),
			Flash:    flash,
		})
		return
	}
	p.host.UpdateVisual(slot.ctxID, VisualPatch{
		Subtitle: new(formatViewerCount(info.ViewerCount)),
		Color:    new(colorTwitch),
		State:    new(1),
		Flash:    flash,
	})
}

func (p *Plugin) cachedBroadcasterID(slot *slotState) (string, bool) {
	ch := strings.ToLower(strings.TrimPrefix(settingString(slot.settings, "channel"), "@"))
	if ch == "" || strings.EqualFold(ch, p.client.Auth().UserLogin) {
		if p.client.Auth().UserID != "" {
			return p.client.Auth().UserID, true
		}
		return "", false
	}
	id, ok := p.client.userIDCache[ch]
	return id, ok
}

func (p *Plugin) slotMatchesBroadcaster(slot *slotState, broadcasterID string) bool {
	bid, ok := p.cachedBroadcasterID(slot)
	return ok && bid == broadcasterID
}

func (p *Plugin) reportSlotError(slot *slotState, fallbackSub string, err error) {
	sub := fallbackSub
	if errors.Is(err, ErrNotAuthenticated) {
		sub = "Sign In (Settings)"
	} else if errors.Is(err, ErrMissingClientID) {
		sub = "Set Client ID"
	}
	p.host.Log(fmt.Sprintf("%s error: %v", slot.actionUUID, err))
	p.host.UpdateVisual(slot.ctxID, VisualPatch{
		Subtitle: new(sub),
		Color:    new(colorErrorState),
		Flash:    "alert",
	})
}

func cloneSettings(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func settingString(settings map[string]any, key string) string {
	if settings == nil {
		return ""
	}
	v, ok := settings[key]
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return strings.TrimSpace(s)
	default:
		return strings.TrimSpace(fmt.Sprint(s))
	}
}

func settingInt(settings map[string]any, key string, fallback int) int {
	if settings == nil {
		return fallback
	}
	v, ok := settings[key]
	if !ok || v == nil {
		return fallback
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case string:
		if iv, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return iv
		}
	}
	return fallback
}

func settingInt64(settings map[string]any, key string) int64 {
	if settings == nil {
		return 0
	}
	v, ok := settings[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case json.Number:
		if iv, err := n.Int64(); err == nil {
			return iv
		}
	case string:
		if iv, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil {
			return iv
		}
	}
	return 0
}

func settingBool(settings map[string]any, key string) bool {
	if settings == nil {
		return false
	}
	v, ok := settings[key]
	if !ok || v == nil {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(strings.TrimSpace(b), "true") || strings.TrimSpace(b) == "1"
	}
	return false
}

func formatViewerCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	case n >= 1_000:
		return fmt.Sprintf("%.2fK", float64(n)/1_000)
	default:
		return strconv.Itoa(n)
	}
}

func formatTimestamp(totalSec int) string {
	if totalSec < 0 {
		totalSec = 0
	}
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func formatDurationMinutes(mins int) string {
	switch {
	case mins >= 43200:
		return fmt.Sprintf("%dmo", mins/43200)
	case mins >= 10080:
		return fmt.Sprintf("%dw", mins/10080)
	case mins >= 1440:
		return fmt.Sprintf("%dd", mins/1440)
	case mins >= 60:
		return fmt.Sprintf("%dh", mins/60)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
}
