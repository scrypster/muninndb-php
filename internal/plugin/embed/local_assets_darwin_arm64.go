//go:build darwin && arm64 && localassets

package embed

import _ "embed"

// nativeLibFilename is the extracted filename for the ORT shared library on this platform.
const nativeLibFilename = "libonnxruntime.dylib"

// embeddedNativeLib is the ORT 1.24.1 shared library for darwin/arm64.
// Populated by `make fetch-assets`.
//
//go:embed assets/libonnxruntime_darwin_arm64.dylib
var embeddedNativeLib []byte
