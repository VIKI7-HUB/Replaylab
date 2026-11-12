package replaylab

import (
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
)

// sseChunkReader yields one OpenAI chat.completion.chunk event per Read, n
// times, then [DONE], the way an upstream delivers a stream.
type sseChunkReader struct {
	n     int
	chunk []byte
	done  bool
}

func (r *sseChunkReader) Read(p []byte) (int, error) {
	if r.n > 0 {
		r.n--
		return copy(p, r.chunk), nil
	}
	if !r.done {
		r.done = true
		return copy(p, "data: [DONE]\n\n"), nil
	}
	return 0, io.EOF
}

// discardWriter is a flushing ResponseWriter that drops what it is sent.
type discardWriter struct{ h http.Header }

func (w *discardWriter) Header() http.Header         { return w.h }
func (w *discardWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *discardWriter) WriteHeader(int)             {}
func (w *discardWriter) Flush()                      {}

// BenchmarkProxyStreamPerChunk reports the cost of relaying one SSE chunk to
// the client, without capture and with capture. Each op is one chunk, so the
// difference between the two is capture's per-chunk overhead on the
// client-write path. Parsing runs on its own goroutine and is not on that
// path.
func BenchmarkProxyStreamPerChunk(b *testing.B) {
	chunk := []byte(`data: {"id":"chatcmpl-bench","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"three word delta "},"finish_reason":null}]}` + "\n\n")

	b.Run("relay", func(b *testing.B) {
		w := &discardWriter{h: http.Header{}}
		b.SetBytes(int64(len(chunk)))
		b.ReportAllocs()
		b.ResetTimer()
		streamCopy(w, &sseChunkReader{n: b.N, chunk: chunk}, nil)
	})

	newCapture := func(b *testing.B) *sseCapture {
		cfg := DefaultConfig()
		cfg.DBPath = filepath.Join(b.TempDir(), "bench.db")
		p, err := NewPlugin(cfg)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = p.Cleanup() })
		if !p.RecordStreamRequest("bench-stream", "", "", "openai", "gpt-4o", `[{"role":"user","content":"hi"}]`, "", "") {
			b.Fatal("stream not captured")
		}
		return newSSECapture(p, "bench-stream")
	}

	// The client-write path with capture on. Chunks go out in batches small
	// enough never to fill the parse backlog, and the timer stops while the
	// parser drains, as it does between real chunks arriving milliseconds
	// apart. Otherwise the relay would outrun the parser, the backlog would
	// overflow and feed would become a cheap no-op.
	b.Run("relay+capture", func(b *testing.B) {
		sc := newCapture(b)
		go sc.run()
		w := &discardWriter{h: http.Header{}}
		const batch = 1000
		b.SetBytes(int64(len(chunk)))
		b.ReportAllocs()
		b.ResetTimer()
		for left := b.N; left > 0; left -= batch {
			streamCopy(w, &sseChunkReader{n: min(batch, left), chunk: chunk, done: true}, sc.feed)
			b.StopTimer()
			for {
				sc.mu.Lock()
				pending, overflow := len(sc.backlog), sc.overflow
				sc.mu.Unlock()
				if overflow {
					b.Fatal("parse backlog overflowed")
				}
				if pending == 0 {
					break
				}
				runtime.Gosched()
			}
			b.StartTimer()
		}
		b.StopTimer()
		sc.finish(false, nil)
	})

	// Parsing one chunk, which happens on the capture goroutine, off the
	// client-write path.
	b.Run("parse", func(b *testing.B) {
		sc := newCapture(b)
		b.SetBytes(int64(len(chunk)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sc.parse(chunk)
		}
	})
}
