// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// sse-choreo-poc is a throwaway component with exactly one job: find out
// whether Choreo's API gateway delivers a Server-Sent Events response
// incrementally as it's written, or buffers it (partially or entirely) and
// only releases it once the connection ends or some buffer threshold is
// hit. If the gateway buffers, SSE is unusable through it regardless of
// anything application code does — worth knowing before relying on it in a
// real feature. See README.md for how to actually test this once deployed.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /stream", handleStream)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	log.Printf("sse-choreo-poc listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// corsAny sets a permissive CORS header so a browser-based test page (e.g.
// an EventSource from a different origin) can actually read the response —
// EventSource is subject to the same-origin policy just like fetch/XHR.
// Fine here since this component is deliberately public and unauthenticated
// already; never do this on anything that carries real auth or session
// state.
func corsAny(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

// handleStream writes a "tick" event every 2s and a comment-only ": ping"
// heartbeat every 15s, flushing after every write, with X-Accel-Buffering
// set — the same technique apps/csm-portal/backend's real
// StreamCaseActivities handler uses. Every tick is logged server-side with
// the time it was sent, so the arrival time a client observes can be
// compared against when the server actually wrote it: a large gap between
// the two means something between here and the client is buffering, not
// this code.
func handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	corsAny(w)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	log.Printf("stream connected: %s", r.RemoteAddr)
	defer log.Printf("stream disconnected: %s", r.RemoteAddr)

	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-tick.C:
			n++
			log.Printf("stream: sending tick %d at %s", n, t.Format(time.RFC3339Nano))
			if _, err := fmt.Fprintf(w, "event: tick\ndata: {\"n\":%d,\"t\":%q}\n\n", n, t.Format(time.RFC3339Nano)); err != nil {
				log.Printf("stream: write failed: %v", err)
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
