package builder

import (
	"errors"
	"os"
	"path/filepath"
)

type buildCache struct {
	dir  string
	prog *cacheProg
}

func newBuildCache(dir string) (*buildCache, error) {
	cache := &buildCache{dir: dir}
	progAndArgs := os.Getenv("GOCACHEPROG")
	if progAndArgs == "" {
		return cache, nil
	}

	prog, err := startCacheProg(progAndArgs)
	if err != nil {
		return nil, err
	}
	cache.prog = prog
	return cache, nil
}

func (c *buildCache) Dir() string {
	return c.dir
}

func (c *buildCache) Path(elem ...string) string {
	parts := append([]string{c.dir}, elem...)
	return filepath.Join(parts...)
}

func (c *buildCache) Close() error {
	if c.prog == nil {
		return nil
	}
	return c.prog.close()
}

func (c *buildCache) Get(kind, key string) (string, bool, error) {
	localPath := c.filePath(kind, key)
	if _, err := os.Stat(localPath); err == nil {
		return localPath, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}

	if c.prog == nil {
		return "", false, nil
	}
	return c.prog.getFile(kind, key)
}

func (c *buildCache) Put(kind, key, tmpPath string) (string, error) {
	if c.prog != nil {
		path, ok, err := c.prog.putFile(kind, key, tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			return "", err
		}
		if ok {
			os.Remove(tmpPath)
			return path, nil
		}
	}

	localPath := c.filePath(kind, key)
	if err := robustRename(tmpPath, localPath); err != nil {
		return "", err
	}
	return localPath, nil
}

func (c *buildCache) filePath(kind, key string) string {
	name := kind + "-" + key
	switch kind {
	case "pkg":
		name += ".bc"
	case "obj":
		name += ".bc"
	case "lib":
		name += ".a"
	case "lib-crt1":
		name += ".o"
	}
	return c.Path(name)
}
