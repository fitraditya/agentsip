package audio

var ulawToLinearTable [256]int16

func init() {
	for i := 0; i < 256; i++ {
		ulawToLinearTable[i] = ulawDecode(byte(i))
	}
}

func ulawDecode(u byte) int16 {
	u = ^u
	sign := u & 0x80
	exp := (u >> 4) & 0x07
	mant := u & 0x0F
	sample := int16(mant<<3+0x84) << exp
	sample -= 0x84 << exp
	if sign == 0 {
		sample = -sample
	}
	return sample
}

func UlawToPCM16(src []byte) []byte {
	dst := make([]byte, len(src)*2)
	for i, b := range src {
		s := ulawToLinearTable[b]
		dst[i*2] = byte(s)
		dst[i*2+1] = byte(s >> 8)
	}
	return dst
}

func UpsamplePCM16x2(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	nSamples := len(src) / 2
	dst := make([]byte, nSamples*2*2)
	for i := 0; i < nSamples; i++ {
		s0 := int16(src[i*2]) | int16(src[i*2+1])<<8
		var s1 int16
		if i+1 < nSamples {
			s1 = int16(src[(i+1)*2]) | int16(src[(i+1)*2+1])<<8
		} else {
			s1 = s0
		}
		mid := int16((int32(s0) + int32(s1)) / 2)
		dst[i*4] = byte(s0)
		dst[i*4+1] = byte(s0 >> 8)
		dst[i*4+2] = byte(mid)
		dst[i*4+3] = byte(mid >> 8)
	}
	return dst
}

func Ulaw8kToPCM16k(src []byte) []byte {
	return UpsamplePCM16x2(UlawToPCM16(src))
}
