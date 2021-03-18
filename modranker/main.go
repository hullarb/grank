package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alixaxel/pagerank"
	"github.com/google/go-github/github"
	"github.com/hullarb/grank/modranker/resolver"
	"golang.org/x/mod/modfile"
)

type mod struct {
	Repo       string
	Path       string
	DirectDeps []string
}

type pkg struct {
	ID          uint32   `json:"id"`
	Name        string   `json:"name"`
	ModuleName  string   `json:"module_name"`
	RepoName    string   `json:"repo_name"`
	Rank        float64  `json:"rank"`
	PRank       int      `json:"prank"`
	GRank       int      `json:"grank"`
	SRank       int      `json:"srank"`
	Stars       int      `json:"stars"`
	Imports     int      `json:"imports"`
	Description string   `json:"description"`
	Topics      []string `json:"topics"`
}

type dependency struct {
	PkgID    uint32 `json:"pkg_id"`
	Upstream bool   `json:"ups"`
}

type dgraph struct {
	Pkgs []pkg                   `json:"pkgs"`
	Deps map[uint32][]dependency `json:"deps"`
}

func (dg dgraph) contains(s, d uint32) bool {
	for _, e := range dg.Deps[s] {
		if e.PkgID == d {
			return true
		}
	}
	return false
}

var (
	nodes     = make(map[string]uint32)
	nodeNames = make(map[uint32]string)
	refs      = make(map[uint32]int)
	w         = make(map[string]int)
	starOrd   = make(map[string]int)
)

var (
	verbose     bool
	downloadDir string
)

func main() {
	rf := flag.String("r", "", "repos json file (produced by lsrepo)")
	of := flag.String("o", "", "output dependency graph file name")
	flag.StringVar(&downloadDir, "d", "repos/", "directory containing the dowloaded github repos")
	flag.BoolVar(&verbose, "v", false, "verbose logs")
	flag.Parse()

	inp, err := os.Open(*rf)
	if err != nil {
		log.Fatal(err)
	}
	defer inp.Close()
	var repos []github.Repository
	err = json.NewDecoder(inp).Decode(&repos)
	if err != nil {
		log.Fatal(err)
	}

	reposByName := map[string]github.Repository{}
	var ord int
	for _, r := range repos {
		rn := "github.com/" + strings.ToLower(r.GetFullName())
		w[rn] = r.GetStargazersCount()
		if _, ok := starOrd[rn]; !ok {
			starOrd[rn] = ord
			ord++
		}
		reposByName[rn] = r
	}

	modules, err := findModules(downloadDir)
	if err != nil {
		log.Printf("failed to list modules in download dir %s: %v", downloadDir, err)
	}

	graph := pagerank.NewGraph()
	var dg dgraph
	dg.Deps = make(map[uint32][]dependency)
	m2r := map[string]string{}
	for _, m := range modules {
		s := nodeID(m.Path)
		m2r[m.Path] = m.Repo
		for _, dn := range m.DirectDeps {
			d := nodeID(dn)
			if dg.contains(s, d) {
				log.Printf("duplicate: %s, %s", m.Path, dn)
				continue
			}
			refs[d]++
			if verbose {
				log.Printf("G: %s -> %s", m.Path, dn)
			}
			graph.Link(s, d, float64(w[m.Repo]))
			dg.Deps[s] = append(dg.Deps[s], dependency{PkgID: d, Upstream: true})
			dg.Deps[d] = append(dg.Deps[d], dependency{PkgID: s})
		}
	}
	probabilityOfFollowingALink := 0.85 // The bigger the number, less probability we have to teleport to some random link
	tolerance := 0.0001                 // the smaller the number, the more exact the result will be but more CPU cycles will be neede

	graph.Rank(probabilityOfFollowingALink, tolerance, func(id uint32, rank float64) {
		name := nodeNames[id]
		rn := m2r[name]
		if rn != "" && rn != name {
			name = fmt.Sprintf("%s (%s)", name, rn)
		} else if rn == "" && strings.HasPrefix(name, "github.com") {
			rn = name
		}
		dg.Pkgs = append(dg.Pkgs, pkg{Name: name, ModuleName: nodeNames[id], RepoName: rn, Rank: rank})
	})
	sort.Slice(dg.Pkgs, func(i, j int) bool {
		return dg.Pkgs[i].Rank > dg.Pkgs[j].Rank
	})
	rank := 1
	prev := .0
	for i, r := range dg.Pkgs {
		repo := reposByName[r.RepoName]
		dg.Pkgs[i].ID = nodes[r.ModuleName]
		dg.Pkgs[i].PRank = rank
		dg.Pkgs[i].SRank = starOrd[r.RepoName]
		dg.Pkgs[i].Stars = w[r.RepoName]
		dg.Pkgs[i].Imports = refs[dg.Pkgs[i].ID]
		if repo.Description != nil {
			dg.Pkgs[i].Description = *repo.Description
		}
		dg.Pkgs[i].Topics = repo.Topics
		r = dg.Pkgs[i]
		fmt.Printf("%d,%d,%d,%s,%v,%d,%d\n", i, r.SRank, r.PRank, r.Name, r.Rank, r.Stars, r.Imports)
		if r.Rank != prev {
			rank++
		}
		prev = r.Rank
	}

	out, err := os.Create(*of)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	err = json.NewEncoder(out).Encode(dg)
	if err != nil {
		log.Fatal(err)
	}
}

func nodeID(nn string) uint32 {
	if id, ok := nodes[nn]; ok {
		return id
	}
	id := uint32(len(nodes))
	nodes[nn] = id
	nodeNames[id] = nn
	return id
}

func findModules(dir string) ([]mod, error) {
	var modules []mod
	moduleFiles := map[string]string{}
	return modules, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("prevent panic by handling failure accessing a path %q: %v\n", path, err)
			return err
		}
		if info.IsDir() || filepath.Base(path) != "go.mod" {
			return nil
		}
		c, err := ioutil.ReadFile(path)
		if err != nil {
			log.Printf("failed to read mod file %s: %v", path, err)
			return nil
		}
		m, err := modfile.ParseLax(path, c, nil)
		if err != nil {
			log.Printf("failed to parse mod file %s: %v", path, err)
			return nil
		}
		if m.Module == nil {
			log.Printf("nil module in %s", path)
			return nil
		}
		pref := downloadDir
		if pref[len(pref)-1] != '/' {
			pref += "/"
		}
		pp := strings.Split(strings.Split(path, pref)[1], "/")
		rd := strings.ToLower(strings.Join(pp[:3], "/"))
		mp := m.Module.Mod.Path
		var direct, nilC int
		if !strings.HasPrefix(strings.ToLower(mp), rd) && !strings.HasPrefix(mp, "github.com") && strings.Contains(strings.Split(mp, "/")[0], ".") {
			log.Printf("path for %s is %s, resolving", mp, rd)
			repo, err := resolver.RepoRootForImportDynamic(mp, resolver.IgnoreMod)
			if err != nil {
				log.Printf("failed to resolve %s: %v", mp, err)
				return nil
			}
			rp := strings.ReplaceAll(repo.Repo, "https://", "")
			rp = strings.ReplaceAll(rp, ".git", "")
			if strings.ToLower(rp) != rd {
				log.Printf("repo for %s is %s while path is %s", mp, rp, rd)
				return nil
			}
		} else if strings.HasPrefix(mp, "github.com") && !strings.HasPrefix(strings.ToLower(mp), rd) {
			log.Printf("module with github path %s is not in expected folder %s", mp, path)
			return nil
		}
		mod := mod{
			Repo: rd,
			Path: mp,
		}
		if p, ok := moduleFiles[mp]; ok {
			log.Printf("found duplicate module file for %s in path %s prev: %s", mp, path, p)
		}
		moduleFiles[mp] = path
		for _, r := range m.Require {
			if r == nil {
				nilC++
				continue
			}
			if !r.Indirect {
				direct++
				mod.DirectDeps = append(mod.DirectDeps, r.Mod.Path)
			}

		}
		modules = append(modules, mod)
		return nil
	})

}
