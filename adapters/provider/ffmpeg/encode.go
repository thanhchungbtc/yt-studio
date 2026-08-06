package ffmpeg

import "runtime"

// encodeArgs is the video encoder every output shares.
//
// On macOS this is the hardware encoder. yuv420p is mandatory there rather than
// cosmetic: VideoToolbox refuses yuvj420p, which is what a JPEG decode yields
// by default, and the encoder then hangs instead of reporting an error.
func encodeArgs() []string {
	if runtime.GOOS == "darwin" {
		return []string{"-c:v", "h264_videotoolbox", "-b:v", "8M", "-pix_fmt", "yuv420p"}
	}
	return []string{"-c:v", "libx264", "-preset", "fast", "-crf", "23", "-pix_fmt", "yuv420p"}
}
