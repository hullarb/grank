// copied and slighltly modified from: https://raw.githubusercontent.com/golang/go/24e5fae92e2c971fa30ac170b7656ff14f3cfde5/src/cmd/go/internal/get/discovery.go

// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resolver

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// charsetReader returns a reader for the given charset. Currently
// it only supports UTF-8 and ASCII. Otherwise, it returns a meaningful
// error which is printed by go get, so the user can find why the package
// wasn't downloaded if the encoding is not supported. Note that, in
// order to reduce potential errors, ASCII is treated as UTF-8 (i.e. characters
// greater than 0x7f are not rejected).
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "ascii":
		return input, nil
	default:
		return nil, fmt.Errorf("can't decode XML document using charset %q", charset)
	}
}

// parseMetaGoImports returns meta imports from the HTML in r.
// Parsing ends at the end of the <head> section or the beginning of the <body>.
func parseMetaGoImports(r io.Reader, mod ModuleMode) (imports []metaImport, err error) {
	d := xml.NewDecoder(r)
	d.CharsetReader = charsetReader
	d.Strict = false
	var t xml.Token
	for {
		t, err = d.RawToken()
		if err != nil {
			if err == io.EOF || len(imports) > 0 {
				err = nil
			}
			break
		}
		if e, ok := t.(xml.StartElement); ok && strings.EqualFold(e.Name.Local, "body") {
			break
		}
		if e, ok := t.(xml.EndElement); ok && strings.EqualFold(e.Name.Local, "head") {
			break
		}
		e, ok := t.(xml.StartElement)
		if !ok || !strings.EqualFold(e.Name.Local, "meta") {
			continue
		}
		// in case of gopkg.in we only find github in go-source
		if attrValue(e.Attr, "name") != "go-import" && attrValue(e.Attr, "name") != "go-source" {
			continue
		}
		if f := strings.Fields(attrValue(e.Attr, "content")); len(f) >= 3 {
			if !strings.Contains(f[2], "github.com") {
				continue
			}
			if attrValue(e.Attr, "name") == "go-source" {
				f[2] = strings.Split(f[2], "/tree/")[0]
			}
			imports = append(imports, metaImport{
				Prefix:   f[0],
				VCS:      f[1],
				RepoRoot: f[2],
			})
		}
	}

	// Extract mod entries if we are paying attention to them.
	var list []metaImport
	var have map[string]bool
	if mod == PreferMod {
		have = make(map[string]bool)
		for _, m := range imports {
			if m.VCS == "mod" {
				have[m.Prefix] = true
				list = append(list, m)
			}
		}
	}

	// Append non-mod entries, ignoring those superseded by a mod entry.
	for _, m := range imports {
		if m.VCS != "mod" && !have[m.Prefix] {
			list = append(list, m)
		}
	}
	return list, nil
}

// attrValue returns the attribute value for the case-insensitive key
// `name', or the empty string if nothing is found.
func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}
