package builder

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheProg(t *testing.T) {
	storeDir := t.TempDir()
	cacheDir := t.TempDir()
	inputPath := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(inputPath, []byte("hello cacheprog"), 0o666); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOCACHEPROG", os.Args[0]+" cacheprog "+storeDir)
	cache, err := newBuildCache(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	path, err := cache.Put("test", "key", inputPath)
	if err != nil {
		t.Fatal(err)
	}
	if path == inputPath {
		t.Fatal("cache program returned the input path instead of a cache path")
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}

	cache, err = newBuildCache(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	path, ok, err := cache.Get("test", "key")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("cache program missed an entry it just stored")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello cacheprog" {
		t.Fatalf("unexpected cache data: %q", data)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
}

func runTestCacheProg(dir string) {
	enc := json.NewEncoder(os.Stdout)
	dec := json.NewDecoder(os.Stdin)
	if err := enc.Encode(cacheProgResponse{
		KnownCommands: []cacheProgCmd{
			cacheProgCmdGet,
			cacheProgCmdPut,
			cacheProgCmdClose,
		},
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for {
		var req cacheProgRequest
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		path := filepath.Join(dir, base64.RawURLEncoding.EncodeToString(req.ActionID))
		switch req.Command {
		case cacheProgCmdGet:
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				enc.Encode(cacheProgResponse{ID: req.ID, Miss: true})
				continue
			}
			if err != nil {
				enc.Encode(cacheProgResponse{ID: req.ID, Err: err.Error()})
				continue
			}
			out := sha256Bytes(data)
			now := time.Now()
			enc.Encode(cacheProgResponse{
				ID:       req.ID,
				OutputID: out,
				Size:     int64(len(data)),
				Time:     &now,
				DiskPath: path,
			})
		case cacheProgCmdPut:
			var body string
			if req.BodySize != 0 {
				if err := dec.Decode(&body); err != nil {
					enc.Encode(cacheProgResponse{ID: req.ID, Err: err.Error()})
					continue
				}
			}
			data, err := base64.StdEncoding.DecodeString(body)
			if err != nil {
				enc.Encode(cacheProgResponse{ID: req.ID, Err: err.Error()})
				continue
			}
			if err := os.WriteFile(path, data, 0o666); err != nil {
				enc.Encode(cacheProgResponse{ID: req.ID, Err: err.Error()})
				continue
			}
			enc.Encode(cacheProgResponse{ID: req.ID, DiskPath: path})
		case cacheProgCmdClose:
			enc.Encode(cacheProgResponse{ID: req.ID})
			return
		default:
			enc.Encode(cacheProgResponse{ID: req.ID, Err: "unknown command"})
		}
	}
}

func sha256Bytes(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}
