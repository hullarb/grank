// functions reused from go source repo: https://github.com/golang/go/blob/da0d1a44bac379f5acedb1933f85400de08f4ac6/src/cmd/go/internal/get/vcs.go

package resolver

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// ModuleMode specifies whether to prefer modules when looking up code sources.
type ModuleMode int

const (
	IgnoreMod ModuleMode = iota
	PreferMod
)

// metaImport represents the parsed <meta name="go-import"
// content="prefix vcs reporoot" /> tags from HTML files.
type metaImport struct {
	Prefix, VCS, RepoRoot string
}

// RepoRoot describes the repository root for a tree of source code.
type RepoRoot struct {
	Repo     string // repository URL, including scheme
	Root     string // import path corresponding to root of repo
	IsCustom bool   // defined by served <meta> tags (as opposed to hard-coded pattern)
	VCS      string // vcs type ("mod", "git", ...)

	// vcs *vcsCmd // internal: vcs command access
}

// repoRootForImportDynamic finds a *RepoRoot for a custom domain that's not
// statically known by repoRootForImportPathStatic.
//
// This handles custom import paths like "name.tld/pkg/foo" or just "name.tld".
func RepoRootForImportDynamic(importPath string, mod ModuleMode) (*RepoRoot, error) {
	slash := strings.Index(importPath, "/")
	if slash < 0 {
		slash = len(importPath)
	}
	host := importPath[:slash]
	if !strings.Contains(host, ".") {
		return nil, errors.New("import path does not begin with hostname")
	}
	urlStr, body, err := GetMaybeInsecure(importPath)
	if err != nil {
		msg := "https fetch: %v"
		// if security == web.Insecure {
		// 	msg = "http/" + msg
		// }
		return nil, fmt.Errorf(msg, err)
	}
	defer body.Close()
	imports, err := parseMetaGoImports(body, mod)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %v", importPath, err)
	}
	// Find the matched meta import.
	mmi, err := matchGoImport(imports, importPath)
	if err != nil {
		if _, ok := err.(ImportMismatchError); !ok {
			return nil, fmt.Errorf("parse %s: %v", urlStr, err)
		}
		return nil, fmt.Errorf("parse %s: no go-import meta tags (%s)", urlStr, err)
	}
	// if cfg.BuildV {
	// 	log.Printf("get %q: found meta tag %#v at %s", importPath, mmi, urlStr)
	// }
	// If the import was "uni.edu/bob/project", which said the
	// prefix was "uni.edu" and the RepoRoot was "evilroot.com",
	// make sure we don't trust Bob and check out evilroot.com to
	// "uni.edu" yet (possibly overwriting/preempting another
	// non-evil student). Instead, first verify the root and see
	// if it matches Bob's claim.
	if mmi.Prefix != importPath {
		// if cfg.BuildV {
		// 	log.Printf("get %q: verifying non-authoritative meta tag", importPath)
		// }
		urlStr0 := urlStr
		var imports []metaImport
		urlStr, imports, err = metaImportsForPrefix(mmi.Prefix, mod)
		if err != nil {
			return nil, err
		}
		metaImport2, err := matchGoImport(imports, importPath)
		if err != nil || mmi != metaImport2 {
			return nil, fmt.Errorf("%s and %s disagree about go-import for %s", urlStr0, urlStr, mmi.Prefix)
		}
	}

	if err := validateRepoRoot(mmi.RepoRoot); err != nil {
		return nil, fmt.Errorf("%s: invalid repo root %q: %v", urlStr, mmi.RepoRoot, err)
	}
	// vcs := vcsByCmd(mmi.VCS)
	// if vcs == nil && mmi.VCS != "mod" {
	// 	return nil, fmt.Errorf("%s: unknown vcs %q", urlStr, mmi.VCS)
	// }

	rr := &RepoRoot{
		Repo:     mmi.RepoRoot,
		Root:     mmi.Prefix,
		IsCustom: true,
		VCS:      mmi.VCS,
		// vcs:      vcs,
	}
	return rr, nil
}

var fetchGroup Group

var (
	fetchCacheMu sync.Mutex
	fetchCache   = map[string]fetchResult{} // key is metaImportsForPrefix's importPrefix
)

type fetchResult struct {
	urlStr  string // e.g. "https://foo.com/x/bar?go-get=1"
	imports []metaImport
	err     error
}

// metaImportsForPrefix takes a package's root import path as declared in a <meta> tag
// and returns its HTML discovery URL and the parsed metaImport lines
// found on the page.
//
// The importPath is of the form "golang.org/x/tools".
// It is an error if no imports are found.
// urlStr will still be valid if err != nil.
// The returned urlStr will be of the form "https://golang.org/x/tools?go-get=1"
func metaImportsForPrefix(importPrefix string, mod ModuleMode) (urlStr string, imports []metaImport, err error) {
	setCache := func(res fetchResult) (fetchResult, error) {
		fetchCacheMu.Lock()
		defer fetchCacheMu.Unlock()
		fetchCache[importPrefix] = res
		return res, nil
	}

	resi, _, _ := fetchGroup.Do(importPrefix, func() (resi interface{}, err error) {
		fetchCacheMu.Lock()
		if res, ok := fetchCache[importPrefix]; ok {
			fetchCacheMu.Unlock()
			return res, nil
		}
		fetchCacheMu.Unlock()

		urlStr, body, err := GetMaybeInsecure(importPrefix)
		if err != nil {
			return setCache(fetchResult{urlStr: urlStr, err: fmt.Errorf("fetch %s: %v", urlStr, err)})
		}
		imports, err := parseMetaGoImports(body, mod)
		if err != nil {
			return setCache(fetchResult{urlStr: urlStr, err: fmt.Errorf("parsing %s: %v", urlStr, err)})
		}
		if len(imports) == 0 {
			err = fmt.Errorf("fetch %s: no go-import meta tag", urlStr)
		}
		return setCache(fetchResult{urlStr: urlStr, imports: imports, err: err})
	})
	res := resi.(fetchResult)
	return res.urlStr, res.imports, res.err
}

// validateRepoRoot returns an error if repoRoot does not seem to be
// a valid URL with scheme.
func validateRepoRoot(repoRoot string) error {
	url, err := url.Parse(repoRoot)
	if err != nil {
		return err
	}
	if url.Scheme == "" {
		return errors.New("no scheme")
	}
	return nil
}

// A ImportMismatchError is returned where metaImport/s are present
// but none match our import path.
type ImportMismatchError struct {
	importPath string
	mismatches []string // the meta imports that were discarded for not matching our importPath
}

func (m ImportMismatchError) Error() string {
	formattedStrings := make([]string, len(m.mismatches))
	for i, pre := range m.mismatches {
		formattedStrings[i] = fmt.Sprintf("meta tag %s did not match import path %s", pre, m.importPath)
	}
	return strings.Join(formattedStrings, ", ")
}

// matchGoImport returns the metaImport from imports matching importPath.
// An error is returned if there are multiple matches.
// errNoMatch is returned if none match.
func matchGoImport(imports []metaImport, importPath string) (metaImport, error) {
	match := -1

	errImportMismatch := ImportMismatchError{importPath: importPath}
	for i, im := range imports {
		if !pathPrefix(importPath, im.Prefix) {
			errImportMismatch.mismatches = append(errImportMismatch.mismatches, im.Prefix)
			continue
		}

		if match >= 0 {
			if imports[match].VCS == "mod" && im.VCS != "mod" {
				// All the mod entries precede all the non-mod entries.
				// We have a mod entry and don't care about the rest,
				// matching or not.
				break
			}
			return metaImport{}, fmt.Errorf("multiple meta tags match import path %q", importPath)
		}
		match = i
		// go-source was included as well, we can expect multiple matches
		break
	}

	if match == -1 {
		return metaImport{}, errImportMismatch
	}
	return imports[match], nil
}

// pathPrefix reports whether sub is a prefix of s,
// only considering entire path components.
func pathPrefix(s, sub string) bool {
	// strings.HasPrefix is necessary but not sufficient.
	if !strings.HasPrefix(s, sub) {
		return false
	}
	// The remainder after the prefix must either be empty or start with a slash.
	rem := s[len(sub):]
	return rem == "" || rem[0] == '/'
}
