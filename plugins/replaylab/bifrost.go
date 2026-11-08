package replaylab

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/maximhq/bifrost/core/schemas"
)

// Replay Lab is a native Bifrost plugin. Register it with the Go SDK:
//
//	lab, _ := replaylab.NewPlugin(replaylab.DefaultConfig())
//	client, _ := bifrost.Init(ctx, schemas.BifrostConfig{
//		Account:    account,
//		LLMPlugins: []schemas.LLMPlugin{lab},
//	})
//
// PreLLMHook records each chat request, PostLLMHook pairs it with the response
// and hands it to the background writer. Neither hook blocks on I/O.
var (
	_ schemas.LLMPlugin           = (*Plugin)(nil)
	_ schemas.HTTPTransportPlugin = (*Plugin)(nil)
)

// Context keys Replay Lab stores on the Bifrost request context.
const (
	ctxKeyCaptureID schemas.BifrostContextKey = "replaylab-capture-id"
	ctxKeyIsReplay  schemas.BifrostContextKey = "replaylab-is-replay"
	ctxKeyTag       schemas.BifrostContextKey = "replaylab-tag"
	ctxKeyTenant    schemas.BifrostContextKey = "replaylab-tenant"
)

// PreRequestHook does not take part in routing.
func (p *Plugin) PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

// PreLLMHook stashes the chat request so PostLLMHook can record it with its
// response. Streaming requests, non-chat requests and Replay Lab's own replays
// are not captured.
func (p *Plugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if req == nil || req.RequestType != schemas.ChatCompletionRequest || req.ChatRequest == nil {
		return req, nil, nil
	}
	if isReplay, _ := ctx.Value(ctxKeyIsReplay).(bool); isReplay {
		return req, nil, nil
	}

	chat := req.ChatRequest
	messages, err := json.Marshal(chat.Input)
	if err != nil {
		return req, nil, nil // never fail a live request because of capture
	}
	var params string
	if chat.Params != nil {
		if b, err := json.Marshal(chat.Params); err == nil && string(b) != "{}" {
			params = string(b)
		}
	}

	id, _ := ctx.Value(schemas.BifrostContextKeyRequestID).(string)
	if id == "" {
		id = "req-" + uuid.NewString()[:12]
	}
	captureID := "bf-" + id // namespaced so it never collides with proxy captures
	ctx.SetValue(ctxKeyCaptureID, captureID)

	vk, _ := ctx.Value(schemas.BifrostContextKeyVirtualKey).(string)
	tag, _ := ctx.Value(ctxKeyTag).(string)
	tenant, _ := ctx.Value(ctxKeyTenant).(string)

	p.RecordRequestWithParams(captureID, vk, tenant, string(chat.Provider), chat.Model, string(messages), params, tag)
	return req, nil, nil
}

// PostLLMHook records the response (or error) for a request captured in
// PreLLMHook. It never modifies what the client receives.
func (p *Plugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	captureID, _ := ctx.Value(ctxKeyCaptureID).(string)
	if captureID == "" {
		return resp, bifrostErr, nil
	}

	if bifrostErr != nil {
		status := http.StatusBadGateway
		if bifrostErr.StatusCode != nil {
			status = *bifrostErr.StatusCode
		}
		msg := "provider error"
		if bifrostErr.Error != nil && bifrostErr.Error.Message != "" {
			msg = bifrostErr.Error.Message
		}
		p.RecordResponse(captureID, "", 0, 0, status, msg, false)
		return resp, bifrostErr, nil
	}

	if resp == nil || resp.ChatResponse == nil {
		p.pendingMap.Delete(captureID)
		return resp, bifrostErr, nil
	}
	chat := resp.ChatResponse
	var prompt, completion int
	if chat.Usage != nil {
		prompt, completion = chat.Usage.PromptTokens, chat.Usage.CompletionTokens
	}
	p.RecordResponse(captureID, chatResponseText(chat), prompt, completion, http.StatusOK, "", false)
	return resp, bifrostErr, nil
}

// chatResponseText returns the first choice's text content.
func chatResponseText(chat *schemas.BifrostChatResponse) string {
	if len(chat.Choices) == 0 || chat.Choices[0].ChatNonStreamResponseChoice == nil {
		return ""
	}
	msg := chat.Choices[0].ChatNonStreamResponseChoice.Message
	if msg == nil || msg.Content == nil {
		return ""
	}
	if msg.Content.ContentStr != nil {
		return *msg.Content.ContentStr
	}
	var sb strings.Builder
	for _, block := range msg.Content.ContentBlocks {
		if block.Text != nil {
			sb.WriteString(*block.Text)
		}
	}
	return sb.String()
}

// HTTPTransportPreAuthHook is unused: Replay Lab needs nothing before auth.
func (p *Plugin) HTTPTransportPreAuthHook(_ *schemas.BifrostContext, _ *schemas.HTTPRequest) (*schemas.HTTPResponse, error) {
	return nil, nil
}

// HTTPTransportPreHook does two things in bifrost-http:
//   - serves the /replaylab/* API from the gateway itself (buffered; use
//     Config.APIListenAddr for live SSE progress)
//   - copies Replay Lab headers onto the request context, so replays are not
//     re-captured and captures carry their tag and tenant.
func (p *Plugin) HTTPTransportPreHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error) {
	if req == nil {
		return nil, nil
	}
	if IsAPIPath(req.Path) {
		path := req.Path
		if len(req.Query) > 0 {
			q := url.Values{}
			for k, v := range req.Query {
				q.Set(k, v)
			}
			path += "?" + q.Encode()
		}
		_, status, headers, body := p.InterceptHTTPRequest(req.Method, path, req.Headers, req.Body)
		return &schemas.HTTPResponse{StatusCode: status, Headers: headers, Body: body}, nil
	}

	if req.CaseInsensitiveHeaderLookup(headerReplay) != "" {
		ctx.SetValue(ctxKeyIsReplay, true)
	}
	if tag := req.CaseInsensitiveHeaderLookup(HeaderTag); tag != "" {
		ctx.SetValue(ctxKeyTag, tag)
	}
	if tenant := req.CaseInsensitiveHeaderLookup(HeaderTenant); tenant != "" {
		ctx.SetValue(ctxKeyTenant, tenant)
	}
	return nil, nil
}

// HTTPTransportPostHook is a no-op.
func (p *Plugin) HTTPTransportPostHook(_ *schemas.BifrostContext, _ *schemas.HTTPRequest, _ *schemas.HTTPResponse) error {
	return nil
}

// HTTPTransportResponseHeadersHook is a no-op.
func (p *Plugin) HTTPTransportResponseHeadersHook(_ *schemas.BifrostContext, _ *schemas.HTTPRequest, _ *schemas.HTTPResponseMetadata) error {
	return nil
}

// HTTPTransportStreamChunkHook passes chunks through unchanged.
func (p *Plugin) HTTPTransportStreamChunkHook(_ *schemas.BifrostContext, _ *schemas.HTTPRequest, chunk *schemas.BifrostStreamChunk) (*schemas.BifrostStreamChunk, error) {
	return chunk, nil
}
