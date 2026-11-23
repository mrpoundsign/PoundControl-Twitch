package twitch

import (
	"encoding/base64"
	"strings"
)

func svgDataURI(svg string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(svg)))
}

// IconPluginLogo returns the PoundControl-Twitch plugin emblem as a data URI.
func IconPluginLogo() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <rect x="6" y="6" width="132" height="132" rx="26" fill="#0A0A0C" stroke="#9146FF" stroke-width="6"/>
  <path d="M35 28 L26 44 V102 H50 V118 L66 102 H86 L116 72 V28 Z" fill="#9146FF"/>
  <path d="M106 68 L88 86 H64 L50 100 V86 H38 V38 H106 Z" fill="#FFFFFF"/>
  <rect x="83" y="50" width="9" height="24" rx="2" fill="#9146FF"/>
  <rect x="62" y="50" width="9" height="24" rx="2" fill="#9146FF"/>
  <circle cx="110" cy="110" r="6" fill="#38BDF8"/>
  <circle cx="94" cy="110" r="6" fill="#38BDF8"/>
  <circle cx="110" cy="94" r="6" fill="#38BDF8"/>
</svg>`)
}

// IconChannelInfo returns the Stream Title & Category edit icon.
func IconChannelInfo() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <rect x="26" y="24" width="66" height="66" rx="10" fill="none" stroke="#FFFFFF" stroke-width="7"/>
  <line x1="40" y1="44" x2="74" y2="44" stroke="#FFFFFF" stroke-width="7" stroke-linecap="round"/>
  <line x1="40" y1="60" x2="64" y2="60" stroke="#FFFFFF" stroke-width="7" stroke-linecap="round"/>
  <line x1="40" y1="76" x2="56" y2="76" stroke="#FFFFFF" stroke-width="7" stroke-linecap="round"/>
  <path d="M74 84 L106 52 L118 64 L86 96 L70 100 Z" fill="#FFFFFF"/>
</svg>`)
}

// IconChatMessage returns the Chat Message speech-bubble icon.
func IconChatMessage() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <path d="M28 28 H116 C121 28 124 31 124 36 V80 C124 85 121 88 116 88 H62 L40 104 V88 H28 C23 88 20 85 20 80 V36 C20 31 23 28 28 28 Z" fill="none" stroke="#FFFFFF" stroke-width="7" stroke-linejoin="round"/>
  <circle cx="52" cy="58" r="6" fill="#FFFFFF"/>
  <circle cx="72" cy="58" r="6" fill="#FFFFFF"/>
  <circle cx="92" cy="58" r="6" fill="#FFFFFF"/>
</svg>`)
}

// IconClearChat returns the Clear Chat speech-bubble with X icon.
func IconClearChat() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <path d="M28 28 H116 C121 28 124 31 124 36 V80 C124 85 121 88 116 88 H62 L40 104 V88 H28 C23 88 20 85 20 80 V36 C20 31 23 28 28 28 Z" fill="none" stroke="#FFFFFF" stroke-width="7" stroke-linejoin="round"/>
  <line x1="56" y1="44" x2="88" y2="72" stroke="#FFFFFF" stroke-width="8" stroke-linecap="round"/>
  <line x1="88" y1="44" x2="56" y2="72" stroke="#FFFFFF" stroke-width="8" stroke-linecap="round"/>
</svg>`)
}

// IconEmoteChat returns the Emote-Only Chat smiley icon.
func IconEmoteChat() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <circle cx="72" cy="58" r="36" fill="none" stroke="#FFFFFF" stroke-width="7"/>
  <circle cx="59" cy="50" r="5" fill="#FFFFFF"/>
  <circle cx="85" cy="50" r="5" fill="#FFFFFF"/>
  <path d="M54 66 C59 77 85 77 90 66" fill="none" stroke="#FFFFFF" stroke-width="7" stroke-linecap="round"/>
</svg>`)
}

// IconFollowersChat returns the Followers-Only Chat heart icon.
func IconFollowersChat() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <path d="M72 94 L38 62 C28 52 28 36 40 28 C50 22 64 26 72 38 C80 26 94 22 104 28 C116 36 116 52 106 62 Z" fill="none" stroke="#FFFFFF" stroke-width="7" stroke-linejoin="round"/>
</svg>`)
}

// IconSubChat returns the Subscribers-Only Chat star icon.
func IconSubChat() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <polygon points="72,20 84,48 114,51 91,71 98,100 72,85 46,100 53,71 30,51 60,48" fill="none" stroke="#FFFFFF" stroke-width="7" stroke-linejoin="round"/>
</svg>`)
}

// IconSlowChat returns the Slow Chat clock icon.
func IconSlowChat() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <circle cx="72" cy="60" r="34" fill="none" stroke="#FFFFFF" stroke-width="7"/>
  <line x1="72" y1="60" x2="72" y2="39" stroke="#FFFFFF" stroke-width="7" stroke-linecap="round"/>
  <line x1="72" y1="60" x2="89" y2="69" stroke="#FFFFFF" stroke-width="7" stroke-linecap="round"/>
  <line x1="60" y1="18" x2="84" y2="18" stroke="#FFFFFF" stroke-width="7" stroke-linecap="round"/>
</svg>`)
}

// IconShieldMode returns the Twitch Shield Mode icon.
func IconShieldMode() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <path d="M72 20 L110 34 V62 C110 83 94 97 72 104 C50 97 34 83 34 62 V34 Z" fill="none" stroke="#FFFFFF" stroke-width="7" stroke-linejoin="round"/>
  <path d="M56 61 L68 73 L90 49" fill="none" stroke="#FFFFFF" stroke-width="7" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`)
}

// IconPlayAd returns the Play Ad broadcast commercial icon.
func IconPlayAd() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <rect x="24" y="26" width="96" height="68" rx="12" fill="none" stroke="#FFFFFF" stroke-width="7"/>
  <polygon points="62,44 62,76 92,60" fill="#FFFFFF"/>
</svg>`)
}

// IconCreateClip returns the Create Clip clapperboard icon.
func IconCreateClip() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <rect x="26" y="42" width="92" height="54" rx="8" fill="none" stroke="#FFFFFF" stroke-width="7"/>
  <path d="M26 42 L114 24 L118 40 L26 56 Z" fill="none" stroke="#FFFFFF" stroke-width="6" stroke-linejoin="round"/>
  <line x1="50" y1="37" x2="44" y2="52" stroke="#FFFFFF" stroke-width="6"/>
  <line x1="76" y1="32" x2="70" y2="47" stroke="#FFFFFF" stroke-width="6"/>
  <line x1="100" y1="27" x2="94" y2="42" stroke="#FFFFFF" stroke-width="6"/>
</svg>`)
}

// IconStreamMarker returns the Stream Marker bookmark flag icon.
func IconStreamMarker() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <path d="M46 22 H98 V92 L72 74 L46 92 Z" fill="none" stroke="#FFFFFF" stroke-width="7" stroke-linejoin="round"/>
  <line x1="26" y1="102" x2="118" y2="102" stroke="#FFFFFF" stroke-width="6" stroke-linecap="round"/>
</svg>`)
}

// IconViewerCount returns the Viewer Count eye icon.
func IconViewerCount() string {
	return svgDataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 144 144" width="144" height="144">
  <path d="M22 60 C34 38 52 28 72 28 C92 28 110 38 122 60 C110 82 92 92 72 92 C52 92 34 82 22 60 Z" fill="none" stroke="#FFFFFF" stroke-width="7" stroke-linejoin="round"/>
  <circle cx="72" cy="60" r="14" fill="#FFFFFF"/>
</svg>`)
}
