package cli

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
)

func TestGatewayHAMTDirectory(t *testing.T) {
	t.Parallel()

	const (
		// The CID of the HAMT-sharded directory that has 10k items
		hamtCid = "bafybeiggvykl7skb2ndlmacg2k5modvudocffxjesexlod2pfvg5yhwrqm"

		// fixtureCid is the CID of root of the DAG that is a subset of hamtCid DAG
		// representing the minimal set of blocks necessary for directory listing.
		// It also includes a "files_refs" file with the list of the references
		// we do NOT needs to fetch (files inside the directory)
		fixtureCid = "bafybeig3yoibxe56aolixqa4zk55gp5sug3qgaztkakpndzk2b2ynobd4i"
	)

	// Start node
	h := harness.NewT(t)
	node := h.NewNode().Init("--empty-repo", "--profile=test").StartDaemon("--offline")
	client := node.GatewayClient()

	// Import fixtures
	r, err := os.Open("./fixtures/TestGatewayHAMTDirectory.car")
	assert.Nil(t, err)
	defer r.Close()
	cid := node.IPFSDagImport(r)
	assert.Equal(t, fixtureCid, cid)

	t.Run("Fetch HAMT directory succeeds with minimal refs", func(t *testing.T) {
		t.Parallel()
		resp := client.Get(fmt.Sprintf("/ipfs/%s/", hamtCid))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Non-minimal refs are not present in the repository", func(t *testing.T) {
		t.Parallel()

		// Fetch list with refs of files that should NOT be available locally.
		resp := client.Get(fmt.Sprintf("/ipfs/%s/files_refs", fixtureCid))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		files := strings.Split(strings.TrimSpace(resp.Body), "\n")
		assert.Len(t, files, 10100)

		// Shuffle the files list and try fetching the first 200.
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })
		for _, cid := range files[:200] {
			resp = client.Get(fmt.Sprintf("/ipfs/%s", cid))
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		}
	})
}

func TestGatewayMultiRange(t *testing.T) {
	t.Parallel()

	const (
		// fileCid is the CID of the large HAMT-sharded file.
		fileCid = "bafybeibkzwf3ffl44yfej6ak44i7aly7rb4udhz5taskreec7qemmw5jiu"

		// fixtureCid is the CID of root of the DAG that is a subset of fileCid DAG
		// representing the minimal set of blocks necessary for a simple byte range request.
		fixtureCid = "bafybeihqs4hdx64a6wmrclp3a2pwxkd5prwdos45bdftpegls5ktzspi7a"
	)

	// Start node
	h := harness.NewT(t)
	node := h.NewNode().Init("--empty-repo", "--profile=test").StartDaemon("--offline")
	client := node.GatewayClient()

	// Import fixtures
	r, err := os.Open("./fixtures/TestGatewayMultiRange.car")
	assert.Nil(t, err)
	defer r.Close()
	cid := node.IPFSDagImport(r)
	assert.Equal(t, fixtureCid, cid)

	t.Run("Succeeds to fetch range of blocks we have", func(t *testing.T) {
		t.Parallel()

		resp := client.Get(fmt.Sprintf("/ipfs/%s", fileCid), func(r *http.Request) {
			r.Header.Set("Range", "bytes=2000-2002, 40000000000-40000000002")
		})
		assert.Equal(t, http.StatusPartialContent, resp.StatusCode)
		assert.Contains(t, resp.Body, "Content-Type: application/octet-stream")
		assert.Contains(t, resp.Body, "Content-Range: bytes 2000-2002/87186935127")
		assert.Contains(t, resp.Body, "Content-Range: bytes 40000000000-40000000002/87186935127")
	})

	t.Run("Fail to fetch range of blocks we do not have", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequest(http.MethodGet, client.BuildURL(fmt.Sprintf("/ipfs/%s", fileCid)), nil)
		assert.Nil(t, err)
		req.Header.Set("Range", "bytes=1000-1100, 87186935125-87186935127")
		httpResp, err := client.Client.Do(req)
		assert.Nil(t, err)
		assert.Equal(t, http.StatusPartialContent, httpResp.StatusCode)
		_, err = io.ReadAll(httpResp.Body)
		assert.Equal(t, err, io.ErrUnexpectedEOF)
	})
}
