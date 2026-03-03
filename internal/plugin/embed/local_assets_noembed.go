//go:build !localassets

package embed

// Stub declarations used when building without embedded assets (no -tags localassets).
// Run `make fetch-assets` then use `-tags localassets` to build with real assets.

var embeddedNativeLib []byte
var embeddedModel []byte
var embeddedTokenizer []byte

const nativeLibFilename = ""

// LocalAvailable reports whether the bundled ONNX model and tokenizer were
// embedded at build time. Always returns false when built without -tags localassets.
func LocalAvailable() bool {
	return false
}
