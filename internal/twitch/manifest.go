package twitch

const (
	PluginUUID = "com.github.mrpoundsign.poundcontrol.twitch"

	ActionChannelInfo   = PluginUUID + ".channel-info"
	ActionChatMessage   = PluginUUID + ".chat-message"
	ActionClearChat     = PluginUUID + ".clear-chat"
	ActionEmoteChat     = PluginUUID + ".emote-chat"
	ActionFollowersChat = PluginUUID + ".followers-chat"
	ActionSubChat       = PluginUUID + ".sub-chat"
	ActionSlowChat      = PluginUUID + ".slow-chat"
	ActionShieldMode    = PluginUUID + ".shield-mode"
	ActionPlayAd        = PluginUUID + ".play-ad"
	ActionCreateClip    = PluginUUID + ".create-clip"
	ActionStreamMarker  = PluginUUID + ".stream-marker"
	ActionViewerCount   = PluginUUID + ".viewer-count"

	SettingClientID     = "clientId"
	SettingAccessToken  = "accessToken"
	SettingRefreshToken = "refreshToken"
	SettingAuthStatus   = "authStatus"
	SettingDeviceCode   = "deviceCode"
	SettingVerifyURL    = "verificationUrl"
	SettingAuthConnect  = "authConnect"
	SettingAuthSignOut  = "authSignOut"

	SettingUserID      = "userId"
	SettingUserLogin   = "userLogin"
	SettingDisplayName = "displayName"
	SettingExpiresAt   = "expiresAt"

	categoryTwitch  = "Twitch"
	colorTwitch     = "#9146FF"
	colorActive     = "#22C55E"
	colorActiveBg   = "#14532D"
	colorShieldOn   = "#EF4444"
	colorShieldBg   = "#5C1922"
	colorInactive   = "#27272A"
	colorErrorState = "#DC2626"
)

// Manifest mirrors PoundControl's GuestManifest ABI.
type Manifest struct {
	UUID                 string         `json:"uuid"`
	Name                 string         `json:"name"`
	Version              string         `json:"version"`
	Author               string         `json:"author,omitempty"`
	Description          string         `json:"description,omitempty"`
	Icon                 string         `json:"icon,omitempty"`
	CategoryIcon         string         `json:"categoryIcon,omitempty"`
	Permissions          Permissions    `json:"permissions,omitzero"`
	GlobalSettingsSchema []SettingField `json:"globalSettingsSchema,omitempty"`
	Actions              []Action       `json:"actions"`
}

// Permissions declares allowed HTTP hosts for the Wasm sandbox.
type Permissions struct {
	AllowedHosts []string `json:"allowedHosts,omitempty"`
}

// Action describes a single Twitch action exposed to PoundControl.
type Action struct {
	UUID           string         `json:"uuid"`
	Name           string         `json:"name"`
	Category       string         `json:"category,omitempty"`
	Tooltip        string         `json:"tooltip,omitempty"`
	Icon           string         `json:"icon,omitempty"`
	DefaultTitle   string         `json:"defaultTitle,omitempty"`
	DefaultColor   string         `json:"defaultColor,omitempty"`
	DefaultImage   string         `json:"defaultImage,omitempty"`
	HideTitle      bool           `json:"hideTitle,omitempty"`
	Controllers    []string       `json:"controllers,omitempty"`
	SettingsSchema []SettingField `json:"settingsSchema,omitempty"`
}

// SelectOption defines one labeled option for a select field.
type SelectOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// SettingField mirrors PoundControl's plugin.SettingField ABI.
type SettingField struct {
	Key         string         `json:"key"`
	Label       string         `json:"label"`
	Type        string         `json:"type"`
	Section     string         `json:"section,omitempty"`
	Default     any            `json:"default,omitempty"`
	Placeholder string         `json:"placeholder,omitempty"`
	Help        string         `json:"help,omitempty"`
	Options     []SelectOption `json:"options,omitempty"`
	Required    bool           `json:"required,omitempty"`
}

func modeField(help string) SettingField {
	return SettingField{
		Key:     "mode",
		Label:   "Press Behavior",
		Type:    "select",
		Section: "General",
		Default: "toggle",
		Help:    help,
		Options: []SelectOption{
			{Label: "Toggle", Value: "toggle"},
			{Label: "Enable (On)", Value: "on"},
			{Label: "Disable (Off)", Value: "off"},
		},
	}
}

func channelField(help string) SettingField {
	return SettingField{
		Key:         "channel",
		Label:       "Target Channel (Optional)",
		Type:        "text",
		Section:     "Advanced",
		Placeholder: "Defaults to your signed-in channel",
		Help:        help,
	}
}

// BuildManifest returns the complete PoundControl Wasm plugin manifest.
func BuildManifest() Manifest {
	return Manifest{
		UUID:         PluginUUID,
		Name:         "Twitch",
		Version:      PluginVersion,
		Author:       "PoundControl",
		Description:  "Full Twitch broadcast, chat, and moderation controls using OAuth2 Device Code authentication",
		Icon:         IconPluginLogo(),
		CategoryIcon: IconPluginLogo(),
		Permissions: Permissions{
			AllowedHosts: []string{
				"id.twitch.tv",
				"api.twitch.tv",
			},
		},
		GlobalSettingsSchema: []SettingField{
			{
				Key:     SettingAuthStatus,
				Label:   "Connection Status",
				Type:    "info",
				Section: "General",
				Default: "Not signed in",
				Help:    "Sign in via Twitch Device Code Flow. OAuth tokens are stored securely in your OS keyring.",
			},
			{
				Key:     SettingDeviceCode,
				Label:   "Activation Code",
				Type:    "info",
				Section: "General",
				Default: "",
				Help:    "Copy this code and enter it on the Twitch activation page.",
			},
			{
				Key:         SettingVerifyURL,
				Label:       "Twitch Activation Page",
				Type:        "link",
				Section:     "General",
				Default:     "",
				Placeholder: "Open twitch.tv/activate",
				Help:        "Click to open Twitch in your browser and authorize PoundControl.",
			},
			{
				Key:     SettingAuthConnect,
				Label:   "Sign In with Twitch",
				Type:    "button",
				Section: "General",
				Default: map[string]any{"label": "Sign In with Twitch", "disabled": false},
			},
			{
				Key:     SettingAuthSignOut,
				Label:   "Sign Out",
				Type:    "button",
				Section: "General",
				Default: map[string]any{"label": "Sign Out", "disabled": true},
			},
			{
				Key:         SettingClientID,
				Label:       "Twitch Client ID",
				Type:        "password",
				Section:     "Advanced",
				Placeholder: "Using built-in Client ID — enter to override",
				Help:        "Pre-configured with PoundControl's public Client ID. Override only if using your own Twitch application; custom IDs are masked and stored in your OS keyring.",
			},
			{
				Key:     SettingAccessToken,
				Label:   "OAuth Access Token",
				Type:    "password",
				Section: "Advanced",
				Help:    "Managed automatically by Device Sign-In and stored in your OS keyring.",
			},
			{
				Key:     SettingRefreshToken,
				Label:   "OAuth Refresh Token",
				Type:    "password",
				Section: "Advanced",
				Help:    "Managed automatically by Device Sign-In and stored in your OS keyring.",
			},
		},
		Actions: []Action{
			{
				UUID:         ActionChannelInfo,
				Name:         "Stream Title & Category",
				Category:     categoryTwitch,
				Tooltip:      "Update your live stream title, game/category, language, or tags",
				DefaultTitle: "Stream Info",
				DefaultColor: colorTwitch,
				DefaultImage: IconChannelInfo(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					{
						Key:         "title",
						Label:       "Stream Title",
						Type:        "text",
						Section:     "General",
						Placeholder: "Leave blank to keep current title",
						Help:        "New broadcast title (up to 140 characters).",
					},
					{
						Key:         "category",
						Label:       "Game / Category",
						Type:        "text",
						Section:     "General",
						Placeholder: "e.g. Just Chatting, Software and Game Development",
						Help:        "Game or category name. Leave blank to keep current category, or enter 'none' to clear.",
					},
					{
						Key:         "language",
						Label:       "Broadcast Language",
						Type:        "text",
						Section:     "Advanced",
						Placeholder: "e.g. en (ISO 639-1, optional)",
					},
					{
						Key:         "tags",
						Label:       "Stream Tags",
						Type:        "text",
						Section:     "Advanced",
						Placeholder: "Comma-separated tags (optional, up to 10)",
					},
				},
			},
			{
				UUID:         ActionChatMessage,
				Name:         "Chat Message",
				Category:     categoryTwitch,
				Tooltip:      "Send a preset message or command to Twitch chat",
				DefaultTitle: "Chat Msg",
				DefaultColor: colorTwitch,
				DefaultImage: IconChatMessage(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					{
						Key:         "message",
						Label:       "Message",
						Type:        "multiline",
						Section:     "General",
						Placeholder: "Hello chat! !discord",
						Help:        "Message text sent to chat (max 500 characters).",
						Required:    true,
					},
					channelField("Target channel login name. Leave blank to send to your own channel."),
				},
			},
			{
				UUID:         ActionClearChat,
				Name:         "Clear Chat",
				Category:     categoryTwitch,
				Tooltip:      "Clear all messages from Twitch chat",
				DefaultTitle: "Clear Chat",
				DefaultColor: colorTwitch,
				DefaultImage: IconClearChat(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					channelField("Channel login to moderate. Leave blank for your own channel."),
				},
			},
			{
				UUID:         ActionEmoteChat,
				Name:         "Emote-Only Chat",
				Category:     categoryTwitch,
				Tooltip:      "Toggle or set Emote-Only chat mode with live status feedback",
				DefaultTitle: "Emote Chat",
				DefaultColor: colorInactive,
				DefaultImage: IconEmoteChat(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					modeField("Toggle switches based on live chat settings."),
					channelField("Channel login to moderate. Leave blank for your own channel."),
				},
			},
			{
				UUID:         ActionFollowersChat,
				Name:         "Followers-Only Chat",
				Category:     categoryTwitch,
				Tooltip:      "Toggle or set Followers-Only chat mode with configurable minimum follow duration",
				DefaultTitle: "Followers",
				DefaultColor: colorInactive,
				DefaultImage: IconFollowersChat(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					modeField("Toggle switches based on live chat settings."),
					{
						Key:     "durationMinutes",
						Label:   "Minimum Follow Duration",
						Type:    "select",
						Section: "General",
						Default: "10",
						Options: []SelectOption{
							{Label: "Any Follower (0 min)", Value: "0"},
							{Label: "10 Minutes", Value: "10"},
							{Label: "30 Minutes", Value: "30"},
							{Label: "1 Hour", Value: "60"},
							{Label: "1 Day", Value: "1440"},
							{Label: "1 Week", Value: "10080"},
							{Label: "1 Month", Value: "43200"},
							{Label: "3 Months", Value: "129600"},
						},
					},
					channelField("Channel login to moderate. Leave blank for your own channel."),
				},
			},
			{
				UUID:         ActionSubChat,
				Name:         "Sub-Only Chat",
				Category:     categoryTwitch,
				Tooltip:      "Toggle or set Subscriber-Only chat mode with live status feedback",
				DefaultTitle: "Sub Chat",
				DefaultColor: colorInactive,
				DefaultImage: IconSubChat(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					modeField("Toggle switches based on live chat settings."),
					channelField("Channel login to moderate. Leave blank for your own channel."),
				},
			},
			{
				UUID:         ActionSlowChat,
				Name:         "Slow Chat",
				Category:     categoryTwitch,
				Tooltip:      "Toggle or set Slow Mode with configurable cooldown seconds",
				DefaultTitle: "Slow Chat",
				DefaultColor: colorInactive,
				DefaultImage: IconSlowChat(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					modeField("Toggle switches based on live chat settings."),
					{
						Key:     "waitSeconds",
						Label:   "Cooldown (seconds)",
						Type:    "select",
						Section: "General",
						Default: "30",
						Options: []SelectOption{
							{Label: "3 seconds", Value: "3"},
							{Label: "5 seconds", Value: "5"},
							{Label: "10 seconds", Value: "10"},
							{Label: "15 seconds", Value: "15"},
							{Label: "30 seconds", Value: "30"},
							{Label: "60 seconds", Value: "60"},
							{Label: "120 seconds", Value: "120"},
						},
					},
					channelField("Channel login to moderate. Leave blank for your own channel."),
				},
			},
			{
				UUID:         ActionShieldMode,
				Name:         "Shield Mode",
				Category:     categoryTwitch,
				Tooltip:      "Instantly activate or deactivate Twitch Shield Mode with live status feedback",
				DefaultTitle: "Shield",
				DefaultColor: colorInactive,
				DefaultImage: IconShieldMode(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					modeField("Toggle switches based on live Shield Mode status."),
					channelField("Channel login to protect. Leave blank for your own channel."),
				},
			},
			{
				UUID:         ActionPlayAd,
				Name:         "Play Ad",
				Category:     categoryTwitch,
				Tooltip:      "Run a commercial break on your live Twitch channel",
				DefaultTitle: "Play Ad",
				DefaultColor: colorTwitch,
				DefaultImage: IconPlayAd(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					{
						Key:     "length",
						Label:   "Ad Duration",
						Type:    "select",
						Section: "General",
						Default: "60",
						Options: []SelectOption{
							{Label: "30 seconds", Value: "30"},
							{Label: "60 seconds", Value: "60"},
							{Label: "90 seconds", Value: "90"},
							{Label: "120 seconds", Value: "120"},
							{Label: "150 seconds", Value: "150"},
							{Label: "180 seconds", Value: "180"},
						},
					},
				},
			},
			{
				UUID:         ActionCreateClip,
				Name:         "Create Clip",
				Category:     categoryTwitch,
				Tooltip:      "Create a clip from your live broadcast (logs clip URL and optionally posts it to chat)",
				DefaultTitle: "Clip",
				DefaultColor: colorTwitch,
				DefaultImage: IconCreateClip(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					{
						Key:     "hasDelay",
						Label:   "Capture Delay",
						Type:    "bool",
						Section: "General",
						Default: false,
						Help:    "Adds a short delay before capturing to account for stream latency.",
					},
					{
						Key:     "postToChat",
						Label:   "Post Clip URL to Chat",
						Type:    "bool",
						Section: "General",
						Default: false,
						Help:    "Automatically sends the created clip link to channel chat.",
					},
					channelField("Target channel login to clip. Leave blank for your own channel."),
				},
			},
			{
				UUID:         ActionStreamMarker,
				Name:         "Stream Marker",
				Category:     categoryTwitch,
				Tooltip:      "Add a timestamp marker to your live broadcast for VOD highlights",
				DefaultTitle: "Marker",
				DefaultColor: colorTwitch,
				DefaultImage: IconStreamMarker(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					{
						Key:         "description",
						Label:       "Marker Description",
						Type:        "text",
						Section:     "General",
						Placeholder: "Highlight moment (optional, max 140 chars)",
					},
				},
			},
			{
				UUID:         ActionViewerCount,
				Name:         "Viewer Count",
				Category:     categoryTwitch,
				Tooltip:      "Displays your live concurrent viewer count and refreshes automatically",
				DefaultTitle: "Viewers",
				DefaultColor: colorTwitch,
				DefaultImage: IconViewerCount(),
				Controllers:  []string{"Keypad"},
				SettingsSchema: []SettingField{
					{
						Key:     "pollSeconds",
						Label:   "Refresh Interval (seconds)",
						Type:    "number",
						Section: "General",
						Default: 30,
						Help:    "How often to refresh viewer count (minimum 10 seconds). Pressing the key refreshes immediately.",
					},
					channelField("Channel login to monitor. Leave blank for your own channel."),
				},
			},
		},
	}
}
