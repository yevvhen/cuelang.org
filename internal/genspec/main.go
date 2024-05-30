// Copyright 2019 CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// genspec regenerates the Hugo markdown language spec from the pinned source of
// truth in the cuelang.org/go module
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	// imported for side effect of module being available in cache
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	_ "cuelang.org/go/pkg"
)

func main() {
	defer handleRunError()

	// Use git to locate the root of the repo which coincides with the
	// root of the Go and CUE modules.
	rootDir := run(".", "git", "rev-parse", "--show-toplevel")

	// Get the "latest" CUE version specified by the site
	ctx := cuecontext.New()
	bis := load.Instances([]string{"."}, &load.Config{
		Dir: rootDir,
	})
	site := ctx.BuildInstance(bis[0])
	if err := site.Err(); err != nil {
		log.Fatal(fmt.Errorf("failed to load site CUE: %w", err))
	}
	latest, err := site.LookupPath(cue.ParsePath("versions.cue.latest.v")).String()
	if err != nil {
		log.Fatal(fmt.Errorf("failed to get latest CUE version: %w", err))
	}

	// Switch to a temporary directory and setup a temporary Go module
	td, err := os.MkdirTemp("", "")
	if err != nil {
		log.Fatal(fmt.Errorf("failed to create a working temp dir: %w", err))
	}
	defer os.RemoveAll(td)
	run(td, "go", "mod", "init", "mod.example")
	run(td, "go", "mod", "edit", "-require", "cuelang.org/go@"+latest)

	// TODO(myitcv): adopt golang.org/issue/44203 when it lands which obviates
	// the need for a temporary module entirely.
	cueDir := run(td, "go", "list", "-m", "-f={{.Dir}}", "cuelang.org/go")
	if cueDir == "" {
		// Not in the modules cache; do a go get and retry. We avoid doing this
		// every time as a minor optimisation, because 'go get' always hits the
		// network regardless of whether there is a precise match in the module
		// (download) cache.
		run(td, "go", "get", "cuelang.org/go@"+latest)
		cueDir = run(td, "go", "list", "-m", "-f={{.Dir}}", "cuelang.org/go")

		// We need a directory at this point
		if cueDir == "" {
			log.Fatal("failed to determine cache directory for cuelang.org/go")
		}
	}

	// We now definitely have a directory for cuelang.org/go within the module cache
	srcSpecPath := filepath.Join(cueDir, "doc", "ref", "spec.md")
	srcSpec, err := os.ReadFile(srcSpecPath)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to read %v: %w", srcSpecPath, err))
	}
	// Strip the prefix upto and including the h1 heading
	const h1 = "# The CUE Language Specification\n"
	_, srcSpec, found := bytes.Cut(srcSpec, []byte(h1))
	if !found {
		log.Fatalf("failed to find h1 heading %q", h1)
	}
	var out bytes.Buffer
	fmt.Fprintln(&out, `---
WARNING: Code generated by internal/genspec; DO NOT EDIT.
title: "The CUE Language Specification"
weight: 10
authors:
- mpvl
aliases:
- /docs/references/spec
---

{{< info >}}
#### Note to implementors
Notes on the formalism underlying this specification can be found
[here](https://github.com/cue-lang/cue/blob/master/doc/ref/impl.md).
{{< /info >}}`)
	out.Write(srcSpec)
	dstPath := "en.md"
	if err := os.WriteFile(dstPath, out.Bytes(), 0666); err != nil {
		log.Fatal(fmt.Errorf("failed to write to %v: %w", dstPath, err))
	}
}

type runError struct {
	cmd *exec.Cmd
	err error
	out []byte
}

func run(dir, cmd string, args ...string) string {
	c := exec.Command(cmd, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		panic(runError{
			cmd: c,
			err: err,
			out: out,
		})
	}
	return strings.TrimSpace(string(out))
}

func handleRunError() {
	switch r := recover().(type) {
	case runError:
		log.Fatal(fmt.Errorf("failed to run [%v]: %w\n%s", r.cmd, r.err, r.out))
	case nil:
		// nothing to do
	default:
		panic(r)
	}
}
