package replaylab

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Headers understood by the capturing proxy.
const (
	HeaderTag        = "X-Replaylab-Tag"    // groups captures (e.g. "json_extraction")
	HeaderTenant     = "X-Replaylab-Tenant" // tenant label stored with the capture
	HeaderVirtualKey = "X-Bf-Vk"            // Bifrost virtual key
	headerReplay     = "X-Replaylab-Replay" // set on replays so they are not re-captured
)

const maxProxyRequestBody = 8 << 20 // 8 MiB

// ProxyHandler returns an OpenAI-compatible /v1/chat/completions handler that
// forwards each request to the configured gateway and records the exchange.
// It lets Replay Lab capture real traffic when it runs as a sidecar in front
// of Bifrost (or any OpenAI-compatible endpoint), not only when embedded.
//
// Streaming requests are forwarded untouched but not captured.
func (p *Plugin) ProxyHandler() http.Handler {
	client := &http.Client{Timeout: 5 * time.Minute}
	upstream := strings.TrimRight(p.config.GatewayURL, "/")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxProxyRequestBody+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		if len(body) > maxProxyRequestBody {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}

		req, captureOK := parseProxyRequest(body)
		capture := captureOK && !req.stream && r.Header.Get(headerReplay) == ""

		reqID := "req-" + uuid.NewString()[:12]
		if capture {
			p.RecordRequestWithParams(reqID, virtualKeyFrom(r), r.Header.Get(HeaderTenant),
				providerOf(req.model), req.model, req.messagesJSON, req.paramsJSON, r.Header.Get(HeaderTag))
		}

		upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstream+r.URL.Path, bytes.NewReader(body))
		if err != nil {
			p.pendingMap.Delete(reqID)
			writeError(w, http.StatusInternalServerError, "failed to build upstream request")
			return
		}
		copyHeaders(upReq.Header, r.Header)

		resp, err := client.Do(upReq)
		if err != nil {
			if capture {
				p.RecordResponse(reqID, "", 0, 0, http.StatusBadGateway, err.Error(), false)
			}
			writeError(w, http.StatusBadGateway, "upstream request failed: "+err.Error())
			return
		}
		defer resp.Body.Close()

		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)

		if !capture {
			streamCopy(w, resp.Body)
			return
		}

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBody))
		_, _ = w.Write(respBody)

		content, prompt, completion, errMsg := "", 0, 0, ""
		if readErr != nil {
			errMsg = readErr.Error()
		} else if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			if parsed, err := parseChatCompletion(respBody); err == nil {
				content, prompt, completion = parsed.content, parsed.promptTokens, parsed.completionTokens
			} else {
				errMsg = err.Error()
			}
		} else {
			errMsg = truncate(string(respBody), errorExcerptLength)
		}
		status := resp.StatusCode
		if errMsg != "" && status < 300 {
			status = http.StatusBadGateway // keep unparseable responses out of replays
		}
		p.RecordResponse(reqID, content, prompt, completion, status, errMsg, false)
	})
}

type proxyRequest struct {
	model        string
	stream       bool
	messagesJSON string
	paramsJSON   string
}

// parseProxyRequest splits a chat request into model, messages and the
// remaining parameters. ok is false when the body is not a chat request.
func parseProxyRequest(body []byte) (proxyRequest, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return proxyRequest{}, false
	}
	var out proxyRequest
	if err := json.Unmarshal(fields["model"], &out.model); err != nil || out.model == "" {
		return proxyRequest{}, false
	}
	msgs, ok := fields["messages"]
	if !ok {
		return proxyRequest{}, false
	}
	out.messagesJSON = string(msgs)
	if s, ok := fields["stream"]; ok {
		_ = json.Unmarshal(s, &out.stream)
	}

	delete(fields, "model")
	delete(fields, "messages")
	delete(fields, "stream")
	delete(fields, "stream_options")
	delete(fields, "user") // end-user identifiers are not needed for replay
	if len(fields) > 0 {
		if b, err := json.Marshal(fields); err == nil {
			out.paramsJSON = string(b)
		}
	}
	return out, true
}

// virtualKeyFrom prefers the Bifrost virtual-key header, then the bearer token.
func virtualKeyFrom(r *http.Request) string {
	if vk := r.Header.Get(HeaderVirtualKey); vk != "" {
		return vk
	}
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// providerOf infers the provider from a Bifrost "provider/model" name.
func providerOf(model string) string {
	if i := strings.Index(model, "/"); i > 0 {
		return model[:i]
	}
	return "openai"
}

var hopByHop = map[string]bool{
	"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true,
	"Proxy-Authorization": true, "Te": true, "Trailer": true,
	"Transfer-Encoding": true, "Upgrade": true, "Content-Length": true,
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		if hopByHop[http.CanonicalHeaderKey(k)] {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// streamCopy relays a response body, flushing as data arrives so SSE
// streams from the upstream reach the client promptly.
func streamCopy(w http.ResponseWriter, body io.Reader) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}
