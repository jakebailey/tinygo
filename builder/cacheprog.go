package builder

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/shlex"
)

type cacheProgCmd string

const (
	cacheProgCmdPut   cacheProgCmd = "put"
	cacheProgCmdGet   cacheProgCmd = "get"
	cacheProgCmdClose cacheProgCmd = "close"
)

type cacheProgRequest struct {
	ID       int64
	Command  cacheProgCmd
	ActionID []byte `json:",omitempty"`
	OutputID []byte `json:",omitempty"`
	BodySize int64  `json:",omitempty"`

	Body io.Reader `json:"-"`
}

type cacheProgResponse struct {
	ID            int64
	Err           string         `json:",omitempty"`
	KnownCommands []cacheProgCmd `json:",omitempty"`

	Miss     bool       `json:",omitempty"`
	OutputID []byte     `json:",omitempty"`
	Size     int64      `json:",omitempty"`
	Time     *time.Time `json:",omitempty"`

	DiskPath string `json:",omitempty"`
}

type cacheProg struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stdin  io.WriteCloser
	bw     *bufio.Writer
	jenc   *json.Encoder

	can map[cacheProgCmd]bool

	closing      atomic.Bool
	ctx          context.Context
	ctxCancel    context.CancelFunc
	readLoopDone chan struct{}

	mu       sync.Mutex
	nextID   int64
	inFlight map[int64]chan<- *cacheProgResponse

	writeMu sync.Mutex
}

func startCacheProg(progAndArgs string) (*cacheProg, error) {
	args, err := shlex.Split(progAndArgs)
	if err != nil {
		return nil, fmt.Errorf("GOCACHEPROG args: %w", err)
	}
	if len(args) == 0 {
		return nil, errors.New("GOCACHEPROG is empty")
	}
	prog := args[0]
	args = args[1:]

	ctx, ctxCancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, prog, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		ctxCancel()
		return nil, fmt.Errorf("StdoutPipe to GOCACHEPROG: %w", err)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		ctxCancel()
		return nil, fmt.Errorf("StdinPipe to GOCACHEPROG: %w", err)
	}
	cmd.Stderr = os.Stderr
	cmd.Cancel = in.Close

	if err := cmd.Start(); err != nil {
		ctxCancel()
		return nil, fmt.Errorf("error starting GOCACHEPROG program %q: %w", prog, err)
	}

	c := &cacheProg{
		cmd:          cmd,
		stdout:       out,
		stdin:        in,
		bw:           bufio.NewWriter(in),
		ctx:          ctx,
		ctxCancel:    ctxCancel,
		readLoopDone: make(chan struct{}),
		inFlight:     make(map[int64]chan<- *cacheProgResponse),
	}
	c.jenc = json.NewEncoder(c.bw)

	capResc := make(chan *cacheProgResponse, 1)
	c.inFlight[0] = capResc
	go c.readLoop()

	for {
		select {
		case res := <-capResc:
			if res == nil {
				c.ctxCancel()
				return nil, errors.New("GOCACHEPROG closed before declaring capabilities")
			}
			if res.Err != "" {
				c.ctxCancel()
				return nil, errors.New(res.Err)
			}
			can := map[cacheProgCmd]bool{}
			for _, cmd := range res.KnownCommands {
				can[cmd] = true
			}
			if len(can) == 0 {
				c.ctxCancel()
				return nil, fmt.Errorf("GOCACHEPROG %v declared no supported commands", prog)
			}
			c.can = can
			return c, nil
		case <-time.After(5 * time.Second):
			fmt.Fprintf(os.Stderr, "# still waiting for GOCACHEPROG %v ...\n", prog)
		}
	}
}

func (c *cacheProg) readLoop() {
	defer close(c.readLoopDone)
	jd := json.NewDecoder(c.stdout)
	for {
		res := new(cacheProgResponse)
		if err := jd.Decode(res); err != nil {
			if c.closing.Load() {
				c.failInFlight(nil)
				return
			}
			c.failInFlight(&cacheProgResponse{
				Err: fmt.Sprintf("error reading JSON from GOCACHEPROG: %v", err),
			})
			return
		}

		c.mu.Lock()
		ch, ok := c.inFlight[res.ID]
		delete(c.inFlight, res.ID)
		c.mu.Unlock()
		if !ok {
			c.failInFlight(&cacheProgResponse{
				Err: fmt.Sprintf("GOCACHEPROG sent response for unknown request ID %v", res.ID),
			})
			return
		}
		ch <- res
	}
}

func (c *cacheProg) failInFlight(res *cacheProgResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.inFlight {
		ch <- res
	}
	c.inFlight = nil
}

var errCacheprogClosed = errors.New("GOCACHEPROG program closed unexpectedly")

func (c *cacheProg) send(ctx context.Context, req *cacheProgRequest) (*cacheProgResponse, error) {
	resc := make(chan *cacheProgResponse, 1)
	if err := c.writeToChild(req, resc); err != nil {
		return nil, err
	}
	select {
	case res := <-resc:
		if res == nil {
			return nil, errCacheprogClosed
		}
		if res.Err != "" {
			return nil, errors.New(res.Err)
		}
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *cacheProg) writeToChild(req *cacheProgRequest, resc chan<- *cacheProgResponse) (err error) {
	c.mu.Lock()
	if c.inFlight == nil {
		c.mu.Unlock()
		return errCacheprogClosed
	}
	c.nextID++
	req.ID = c.nextID
	c.inFlight[req.ID] = resc
	c.mu.Unlock()

	defer func() {
		if err != nil {
			c.mu.Lock()
			if c.inFlight != nil {
				delete(c.inFlight, req.ID)
			}
			c.mu.Unlock()
		}
	}()

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.jenc.Encode(req); err != nil {
		return err
	}
	if err := c.bw.WriteByte('\n'); err != nil {
		return err
	}
	if req.Body != nil && req.BodySize > 0 {
		if err := c.bw.WriteByte('"'); err != nil {
			return err
		}
		e := base64.NewEncoder(base64.StdEncoding, c.bw)
		wrote, err := io.Copy(e, req.Body)
		if err != nil {
			return err
		}
		if err := e.Close(); err != nil {
			return err
		}
		if wrote != req.BodySize {
			return fmt.Errorf("short write writing body to GOCACHEPROG for action %x, output %x: wrote %v; expected %v",
				req.ActionID, req.OutputID, wrote, req.BodySize)
		}
		if _, err := c.bw.WriteString("\"\n"); err != nil {
			return err
		}
	}
	return c.bw.Flush()
}

func (c *cacheProg) getFile(kind, key string) (string, bool, error) {
	if !c.can[cacheProgCmdGet] {
		return "", false, nil
	}
	res, err := c.send(c.ctx, &cacheProgRequest{
		Command:  cacheProgCmdGet,
		ActionID: cacheProgActionID(kind, key),
	})
	if err != nil {
		return "", false, err
	}
	if res.Miss {
		return "", false, nil
	}
	if res.DiskPath == "" {
		return "", false, errors.New("GOCACHEPROG did not populate DiskPath on get hit")
	}
	info, err := os.Stat(res.DiskPath)
	if err != nil {
		return "", false, fmt.Errorf("GOCACHEPROG returned unusable DiskPath: %w", err)
	}
	if info.Size() != res.Size {
		return "", false, errors.New("GOCACHEPROG returned incomplete DiskPath")
	}
	return res.DiskPath, true, nil
}

func (c *cacheProg) putFile(kind, key, path string) (string, bool, error) {
	if !c.can[cacheProgCmdPut] {
		return "", false, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", false, err
	}

	h := sha256.New()
	size, err := io.Copy(h, file)
	if err != nil {
		return "", false, err
	}
	if size != info.Size() {
		return "", false, fmt.Errorf("file size changed while storing in GOCACHEPROG: %s", path)
	}
	var out [sha256.Size]byte
	h.Sum(out[:0])
	if _, err := file.Seek(0, 0); err != nil {
		return "", false, err
	}

	res, err := c.send(c.ctx, &cacheProgRequest{
		Command:  cacheProgCmdPut,
		ActionID: cacheProgActionID(kind, key),
		OutputID: out[:],
		Body:     file,
		BodySize: size,
	})
	if err != nil {
		return "", false, err
	}
	if res.DiskPath == "" {
		return "", false, errors.New("GOCACHEPROG did not return DiskPath in put response")
	}
	return res.DiskPath, true, nil
}

func (c *cacheProg) close() error {
	c.closing.Store(true)
	var err error
	if c.can[cacheProgCmdClose] {
		_, err = c.send(c.ctx, &cacheProgRequest{Command: cacheProgCmdClose})
		if errors.Is(err, errCacheprogClosed) {
			err = nil
		}
	}
	c.ctxCancel()
	<-c.readLoopDone
	return err
}

func cacheProgActionID(kind, key string) []byte {
	h := sha256.New()
	h.Write([]byte("tinygo build cache v1\n"))
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(key))
	return h.Sum(nil)
}
