package sip

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/pion/sdp/v3"
)

// ErrUnsupportedOffer means the offer isn't a single-audio-stream call this
// adapter knows how to answer, or doesn't include PCMU.
var ErrUnsupportedOffer = errors.New("sip: offer is not a single-audio-stream this adapter can answer")

// payloadTypePCMU is RFC 3551's static assignment for G.711 u-law — the one
// codec this adapter answers with (see buildAnswer).
const payloadTypePCMU = "0"

func parseAudioOffer(body []byte) (*sdp.SessionDescription, *sdp.MediaDescription, error) {
	var sd sdp.SessionDescription
	if err := sd.Unmarshal(body); err != nil {
		return nil, nil, fmt.Errorf("sip: parse sdp offer: %w", err)
	}

	var audio *sdp.MediaDescription
	for _, m := range sd.MediaDescriptions {
		if m.MediaName.Media == "audio" {
			audio = m
			break
		}
	}
	if audio == nil {
		return nil, nil, fmt.Errorf("%w: no audio stream", ErrUnsupportedOffer)
	}
	return &sd, audio, nil
}

// buildAnswer produces an SDP answer that accepts only PCMU, pointed at our
// own RTP socket. Narrowing to one codec rather than mirroring whatever the
// caller offered keeps this adapter (and every future STT/TTS adapter reading
// its audio) from needing to handle more than one wire format for v1.
//
// This mutates offer/audio in place and marshals the result, following the
// same "rewrite the offer into an answer" technique as obrelin's media relay,
// since SDP answers otherwise need a large amount of boilerplate to build
// from scratch correctly.
func buildAnswer(offer *sdp.SessionDescription, audio *sdp.MediaDescription, advertiseIP string, rtpPort int) ([]byte, error) {
	if !hasFormat(audio, payloadTypePCMU) {
		return nil, fmt.Errorf("%w: caller did not offer PCMU", ErrUnsupportedOffer)
	}

	offer.ConnectionInformation = &sdp.ConnectionInformation{
		NetworkType: "IN",
		AddressType: "IP4",
		Address:     &sdp.Address{Address: advertiseIP},
	}
	offer.Origin.UnicastAddress = advertiseIP

	// A leftover media-level c= line would override the session-level one just
	// set above and point media at the wrong address.
	audio.ConnectionInformation = nil
	audio.MediaName.Port = sdp.RangedPort{Value: rtpPort}
	audio.MediaName.Formats = []string{payloadTypePCMU}
	audio.Attributes = keepAttributesForFormat(audio.Attributes, payloadTypePCMU)

	out, err := offer.Marshal()
	if err != nil {
		return nil, fmt.Errorf("sip: marshal sdp answer: %w", err)
	}
	return out, nil
}

func hasFormat(audio *sdp.MediaDescription, pt string) bool {
	return slices.Contains(audio.MediaName.Formats, pt)
}

// keepAttributesForFormat drops the rtpmap/fmtp/rtcp-fb attributes belonging
// to payload types no longer in the m= line — an attribute left behind for a
// format that's gone is at best ignored and at worst read as still offered.
func keepAttributesForFormat(attrs []sdp.Attribute, pt string) []sdp.Attribute {
	var out []sdp.Attribute
	for _, a := range attrs {
		switch a.Key {
		case "rtpmap", "fmtp", "rtcp-fb":
			attrPT, _, found := strings.Cut(a.Value, " ")
			if found && strings.TrimSpace(attrPT) != pt {
				continue
			}
		}
		out = append(out, a)
	}
	return out
}
