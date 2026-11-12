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
// The client's request is forwarded unmodified. Streamed responses
// ("stream": true) are relayed to the client chunk by chunk as they arrive and
// parsed on the side (see sseCapture), so capture never delays or alters what
// the client receives.
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
		capture := captureOK && r.Header.Get(headerReplay) == ""

		reqID := "req-" + uuid.NewString()[:12]
		if capture {
			vk, tenant, tag := virtualKeyFrom(r), r.Header.Get(HeaderTenant), r.Header.Get(HeaderTag)
			if req.stream {
				capture = p.RecordStreamRequest(reqID, vk, tenant, providerOf(req.model), req.model, req.messagesJSON, req.paramsJSON, tag)
			} else {
				p.RecordRequestWithParams(reqID, vk, tenant, providerOf(req.model), req.model, req.messagesJSON, req.paramsJSON, tag)
			}
		}
		streamed := capture && req.stream

		upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstream+r.URL.Path, bytes.NewReader(body))
		if err != nil {
			p.pendingMap.Delete(reqID)
			writeError(w, http.StatusInternalServerError, "failed to build upstream request")
			return
		}
		copyHeaders(upReq.Header, r.Header)

		resp, err := client.Do(upReq)
		if err != nil {
			switch {
			case streamed && r.Context().Err() != nil:
				p.FinishStream(reqID, StreamOutcome{State: CompletionStateAbandoned, ErrorMessage: "client disconnected before the stream started"})
			case streamed:
				p.FinishStream(reqID, StreamOutcome{State: CompletionStateComplete, StatusCode: http.StatusBadGateway, ErrorMessage: err.Error()})
			case capture:
				p.RecordResponse(reqID, "", 0, 0, http.StatusBadGateway, err.Error(), false)
			}
			writeError(w, http.StatusBadGateway, "upstream request failed: "+err.Error())
			return
		}
		defer resp.Body.Close()

		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)

		if !capture {
			streamCopy(w, resp.Body, nil)
			return
		}
		if streamed && isSSEResponse(resp) {
			sc := newSSECapture(p, reqID)
			go sc.run()
			clientGone, readErr := streamCopy(w, resp.Body, sc.feed)
			if !clientGone && readErr != nil && r.Context().Err() != nil {
				clientGone = true // the read failed because the client went away
			}
			sc.finish(clientGone, readErr)
			return
		}

		// A non-streamed exchange, or a stream request the upstream answered
		// with a plain body (usually an error): relay it whole, then record it.
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBody))
		_, _ = w.Write(respBody)
		content, prompt, completion, status, errMsg := bufferedOutcome(resp.StatusCode, respBody, readErr)
		if streamed {
			p.AppendStreamContent(reqID, content)
			p.FinishStream(reqID, StreamOutcome{
				State: CompletionStateComplete, StatusCode: status, ErrorMessage: errMsg,
				HasUsage: errMsg == "", PromptTokens: prompt, CompletionTokens: completion,
			})
			return
		}
		p.RecordResponse(reqID, content, prompt, completion, status, errMsg, false)
	})
}

// bufferedOutcome interprets a whole upstream response body.
func bufferedOutcome(statusCode int, respBody []byte, readErr error) (content string, prompt, completion, status int, errMsg string) {
	if readErr != nil {
		errMsg = readErr.Error()
	} else if statusCode >= 200 && statusCode <= 299 {
		if parsed, err := parseChatCompletion(respBody); err == nil {
			content, prompt, completion = parsed.content, parsed.promptTokens, parsed.completionTokens
		} else {
			errMsg = err.Error()
		}
	} else {
		errMsg = truncate(string(respBody), errorExcerptLength)
	}
	status = statusCode
	if errMsg != "" && status < 300 {
		status = http.StatusBadGateway // keep unparseable responses out of replays
	}
	return content, prompt, completion, status, errMsg
}

// isSSEResponse reports whether the upstream answered with a successful
// event stream.
func isSSEResponse(resp *http.Response) bool {
	return resp.StatusCode >= 200 && resp.StatusCode <= 299 &&
		strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
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
// streams from the upstream reach the client promptly. observe, when not nil,
// is handed each relayed chunk only after it has been written and flushed;
// it must not retain the slice. streamCopy reports whether the client write
// failed, and any upstream read error other than io.EOF.
func streamCopy(w http.ResponseWriter, body io.Reader, observe func([]byte)) (clientGone bool, readErr error) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return true, nil
			}
			if flusher != nil {
				flusher.Flush()
			}
			if observe != nil {
				observe(buf[:n])
			}
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}
