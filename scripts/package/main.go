// Package Linux archives with portable executable permissions, including when
// the release is built on Windows.
package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	source := flag.String("source", "", "bundle directory")
	output := flag.String("output", "", "tar.gz path")
	flag.Parse()
	if *source == "" || *output == "" {
		return fmt.Errorf("source and output are required")
	}
	f, err := os.Create(*output)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	err = filepath.WalkDir(*source, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		h, e := tar.FileInfoHeader(info, "")
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(filepath.Dir(*source), path)
		if e != nil {
			return e
		}
		h.Name = filepath.ToSlash(rel)
		h.Mode = 0644
		if d.IsDir() || d.Name() == "nlroom-cli" || d.Name() == "nlroom-service" || d.Name() == "nlroom-update" || d.Name() == "nlroom-node" || d.Name() == "nodelane-server" {
			h.Mode = 0755
		}
		if e = tw.WriteHeader(h); e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		r, e := os.Open(path)
		if e != nil {
			return e
		}
		defer r.Close()
		_, e = io.Copy(tw, r)
		return e
	})
	if err != nil {
		return err
	}
	if err = tw.Close(); err != nil {
		return err
	}
	if err = gz.Close(); err != nil {
		return err
	}
	return f.Close()
}
