package builder

import (
	"errors"
	"os"
	"path/filepath"
)

type buildCache struct {
	dir string
}

func newBuildCache(dir string) (*buildCache, error) {
	return &buildCache{dir: dir}, nil
}

func (c *buildCache) Dir() string {
	return c.dir
}

func (c *buildCache) Path(elem ...string) string {
	parts := append([]string{c.dir}, elem...)
	return filepath.Join(parts...)
}

func (c *buildCache) Close() error {
	return nil
}

func (c *buildCache) Get(kind, key string) (string, bool, error) {
	localPath := c.filePath(kind, key)
	if _, err := os.Stat(localPath); err == nil {
		return localPath, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	return "", false, nil
}

func (c *buildCache) Put(kind, key, tmpPath string) (string, error) {
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
