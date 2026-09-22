// Package typesafe is a client for the TypeSafe AI System One API.
//
// Send unstructured state and a map of typed questions; receive calibrated,
// structured answers. Build a Client with New, ask with Client.SystemOne,
// and list the account's models with Client.Models.
//
// The package logs nothing by default, never logs request or response
// bodies (state is customer data), and returns errors rather than logging
// them. Wrap Config.HTTPClient with a logging or tracing RoundTripper for
// wire-level visibility.
package typesafe

// Version is the SDK version sent in the User-Agent and X-TypeSafe-SDK
// headers. Bumped on tag.
const Version = "0.1.0"
