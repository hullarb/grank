package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/google/go-github/github"

	"github.com/alixaxel/pagerank"
)

type pkg struct {
	ID          uint32   `json:"id"`
	Name        string   `json:"name"`
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
	verbose bool
	srcPref string
)

func main() {
	rf := flag.String("r", "", "repos json file (produced by lsrepo)")
	df := flag.String("d", "", "dependencies csv file (produced by buildg)")
	ruf := flag.String("ru", "", "pkg repo url mapping file (produced by resolver)")
	of := flag.String("o", "", "output dependency graph file name")
	so := flag.Bool("s", false, "create also an output with small- prefix containing only the top few hundred repos for testing")
	flag.StringVar(&srcPref, "pref", "grank/download/", "directory containing the dowloaded github repos relative to GOPATH")
	flag.BoolVar(&verbose, "v", false, "verbose logs")
	flag.Parse()

	d, err := os.Open(*df)
	if err != nil {
		log.Fatal(err)
	}

	deps, err := dGraph(d)
	if err != nil {
		log.Fatal(err)
	}

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

	repoURLs := map[string]string{}
	if *ruf != "" {
		ns, err := os.Open(*ruf)
		if err != nil {
			log.Fatal(err)
		}
		err = json.NewDecoder(ns).Decode(&repoURLs)
		if err != nil {
			log.Fatal(err)
		}
	}

	graph := pagerank.NewGraph()
	var dg dgraph
	dg.Deps = make(map[uint32][]dependency)
	for sn, edges := range deps {
		for dn := range edges {
			s := nodeID(sn)
			if pn, ok := repoURLs[dn]; ok {
				dn = pn
			}
			dn = repo(dn)
			if dn == "" {
				panic("invalid github repo")
			}
			d := nodeID(dn)
			if s == d {
				log.Printf("source and dst the same: %s, %s", sn, dn)
				continue
			}
			if dg.contains(s, d) {
				log.Printf("duplicate: %s, %s", sn, dn)
				continue
			}
			refs[d]++
			if verbose {
				log.Printf("G: %s -> %s", sn, dn)
			}
			graph.Link(s, d, float64(w[sn]))
			dg.Deps[s] = append(dg.Deps[s], dependency{PkgID: d, Upstream: true})
			dg.Deps[d] = append(dg.Deps[d], dependency{PkgID: s})
		}
	}

	probabilityOfFollowingALink := 0.85 // The bigger the number, less probability we have to teleport to some random link
	tolerance := 0.0001                 // the smaller the number, the more exact the result will be but more CPU cycles will be neede

	graph.Rank(probabilityOfFollowingALink, tolerance, func(id uint32, rank float64) {
		dg.Pkgs = append(dg.Pkgs, pkg{Name: nodeNames[id], Rank: rank})
	})
	sort.Slice(dg.Pkgs, func(i, j int) bool {
		return dg.Pkgs[i].Rank > dg.Pkgs[j].Rank
	})
	rank := 1
	grank := 0
	prev := .0
	for i, r := range dg.Pkgs {
		dg.Pkgs[i].GRank = -1
		if strings.Contains(r.Name, "github.com/") {
			dg.Pkgs[i].GRank = grank
			if r.Rank != prev {
				grank++
			}
		}
		repo := reposByName[r.Name]
		dg.Pkgs[i].ID = nodes[r.Name]
		dg.Pkgs[i].PRank = rank
		dg.Pkgs[i].SRank = starOrd[r.Name]
		dg.Pkgs[i].Stars = w[r.Name]
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

	var sdg dgraph
	sdg.Deps = make(map[uint32][]dependency)
	fi := map[uint32]struct{}{}
	for i, r := range dg.Pkgs {
		if i > 250 {
			break
		}
		fi[r.ID] = struct{}{}
		sdg.Pkgs = append(sdg.Pkgs, r)
	}
	for _, r := range sdg.Pkgs {
		for _, d := range dg.Deps[r.ID] {
			if _, ok := fi[d.PkgID]; !ok {
				continue
			}
			sdg.Deps[r.ID] = append(sdg.Deps[r.ID], d)
		}
	}

	if *so {
		sout, err := os.Create("small-" + *of)
		if err != nil {
			log.Fatal(err)
		}
		defer sout.Close()
		err = json.NewEncoder(sout).Encode(sdg)
		if err != nil {
			log.Fatal(err)
		}
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

func dGraph(depsF io.Reader) (map[string]map[string]struct{}, error) {
	deps := bufio.NewReader(depsF)
	g := make(map[string]map[string]struct{})
	for {
		line, err := deps.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read deps: %v", err)
		}
		parts := strings.Split(strings.TrimRight(line, "\n"), ",")

		s := cleanSrc(parts[0])
		if s == "" {
			return nil, fmt.Errorf("malformd src pkg name: %s", parts[0])
		}
		d := clean(parts[1])
		d = repo(d)
		if d == "" {
			log.Printf("invalid github repo: %s", parts[1])
			continue
		}
		e := g[s]
		if e == nil {
			e = make(map[string]struct{})
		}
		e[d] = struct{}{}
		g[s] = e
	}
	return g, nil
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

func cleanSrc(p string) string {
	parts := strings.Split(strings.Replace(p, srcPref, "", 1), "/")
	if len(parts) < 3 {
		return ""
	}
	return strings.ToLower(strings.Join(parts[0:3], "/"))
}

func clean(d string) string {
	d = strings.ToLower(d)
	parts := strings.Split(d, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "src" || parts[i] == "vendor" || parts[i] == "_vendor" || strings.ToLower(parts[i]) == "c" {
			return strings.Join(parts[i+1:], "/")
		}
	}
	return d
}

func repo(d string) string {
	if !strings.HasPrefix(d, "github.com") {
		return d
	}
	parts := strings.Split(d, "/")
	if len(parts) < 3 {
		return ""
	}
	return strings.Join(parts[0:3], "/")
}
