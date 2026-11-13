//go:build !replaylab_core_v1_7

package replaylab

import "github.com/maximhq/bifrost/core/schemas"

// Bifrost core v1.7.x has no HTTPResponseMetadata and no response-headers
// hook. The loadable plugin (loadable/) targets a bifrost-http release built
// on that core, so it builds with -tags replaylab_core_v1_7 and this file drops
// out. See docs/adr/0001-loadable-plugin-target-gateway.md.

var _ schemas.HTTPTransportPlugin = (*Plugin)(nil)

// HTTPTransportResponseHeadersHook is a no-op.
func (p *Plugin) HTTPTransportResponseHeadersHook(_ *schemas.BifrostContext, _ *schemas.HTTPRequest, _ *schemas.HTTPResponseMetadata) error {
	return nil
}
