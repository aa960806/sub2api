package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Public Canvas JS -> real HTTP handlers -> real Seedance provider -> mock
// supplier. The fixture asserts upstream auth/protocol and downstream download.
func TestSeedanceCanvasClientContract(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js required to replay the public Canvas client")
	}
	e := setupCompatEnv(t)
	var creates, uploads atomic.Int32
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/files":
			var body map[string]string
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, compatPNG, body["image_b64"])
			n := uploads.Add(1)
			w.WriteHeader(201)
			fmt.Fprintf(w, `{"image_id":"ref-%d","expires_at":%d}`, n, time.Now().Add(time.Hour).Unix())
		case r.Method == "POST" && r.URL.Path == "/v1/videos":
			var body seedanceCreateBody
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))
			assert.Equal(t, "seedance2.0", body.Model)
			assert.Equal(t, 5, body.Duration)
			assert.Equal(t, "720p", body.Resolution)
			if strings.Contains(body.Prompt, "图片1") {
				assert.Equal(t, []string{"ref-1", "ref-2"}, body.ImageIDs)
			}
			n := creates.Add(1)
			w.WriteHeader(202)
			fmt.Fprintf(w, `{"id":"job-%d","status":"queued"}`, n)
		case strings.HasSuffix(r.URL.Path, "/signed_url"):
			fmt.Fprintf(w, `{"url":"%s/file","expires_at":%d}`, strings.TrimSuffix(r.URL.Path, "/signed_url"), time.Now().Add(time.Minute).Unix())
		case strings.HasSuffix(r.URL.Path, "/file"):
			w.Header().Set("Content-Type", "video/mp4")
			io.WriteString(w, "MP4-FIXTURE")
		case strings.HasPrefix(r.URL.Path, "/v1/videos/"):
			fmt.Fprintf(w, `{"id":%q,"status":"succeeded"}`, strings.TrimPrefix(r.URL.Path, "/v1/videos/"))
		default:
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
	account.ID = 1
	account.Status = "active"
	account.Schedulable = true
	account.GroupIDs = []int64{9}
	account.Concurrency = 10
	e.resolver.accounts[1] = account
	e.handler.tasks.provider = provider
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/media/results/") && r.Header.Get("Authorization") != "Bearer canvas-fixture-key" {
			w.WriteHeader(401)
			return
		}
		// Run the same worker deterministically instead of sleeping for its tick.
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, compatPath+"/") {
			if err := e.handler.advanceTask(r.Context(), strings.TrimPrefix(r.URL.Path, compatPath+"/"), false); err != nil {
				t.Errorf("worker: %v", err)
			}
		}
		e.router.ServeHTTP(w, r)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, node, "testdata/canvas/verify.cjs", server.URL).CombinedOutput()
	require.NoError(t, err, string(output))
	t.Log(string(output))
	require.EqualValues(t, 2, creates.Load())
	require.EqualValues(t, 2, uploads.Load())
	billing := e.handler.tasks.Billing().(*lifecycleBilling)
	require.Equal(t, 4.0, billing.charged)
	require.Zero(t, billing.frozen)
}
