package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var (
	all     int
	found   map[string]struct{}
	client  *github.Client
	jsonEnc *json.Encoder
	out     *os.File
)

func main() {
	log.SetFlags(log.Lmicroseconds)
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		fmt.Println("GH_TOKEN env var has to contain a valid github api access token")
		os.Exit(1)
	}
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./lsrepo out_file_name [older repos json files]")
		fmt.Println()
		fmt.Println("out_file_name: a file with the name will be created with a json array of the fetched github repos")
		fmt.Println("older repos json files: results of earlier runs to ensure that all the repositoreis from those files are fetched")
		os.Exit(1)
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	ctx := context.Background()
	tc := oauth2.NewClient(ctx, ts)
	client = github.NewClient(tc)
	found = map[string]struct{}{}

	var err error
	out, err = os.Create(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	_, err = out.WriteString("[")
	if err != nil {
		log.Fatal(err)
	}
	jsonEnc = json.NewEncoder(out)
	var lastStars int
	for {
		q := "language:go"
		if lastStars != 0 {
			q += fmt.Sprintf(" stars:<=%v", lastStars)
		}
		l := getAllPages(q)
		if l == lastStars {
			log.Printf("last start count %d did not change, exiting", lastStars)
			break
		}
		lastStars = l
	}
	log.Printf("fetched %d repos from API", all)
	fetchMissing(os.Args[2:])
	_, err = out.WriteString("]")
	if err != nil {
		log.Fatal(err)
	}
}

func fetchMissing(repoFiles []string) {
	for _, f := range repoFiles {
		inp, err := os.Open(f)
		if err != nil {
			log.Fatalf("failed to open: %v", err)
		}
		var rs []github.Repository
		err = json.NewDecoder(inp).Decode(&rs)
		if err != nil {
			log.Fatalf("failed to decode: %v", err)
		}
		for _, r := range rs {
			if _, ok := found[r.GetFullName()]; ok {
				continue
			}
			log.Printf("fetching missing: %s", r.GetFullName())
			for {
				rp, resp, err := client.Repositories.Get(context.Background(), r.GetOwner().GetLogin(), r.GetName())
				if err != nil {
					if _, ok := err.(*github.RateLimitError); ok {
						log.Println("hit rate limit")
						time.Sleep(10 * time.Second)
						continue
					} else {
						log.Fatalf("failed to fetch %s: %v", r.GetFullName(), err)
					}
				}
				if _, err = out.Write([]byte{',', '\n'}); err != nil {
					log.Fatal(err)
				}
				if err = jsonEnc.Encode(rp); err != nil {
					log.Fatal(err)
				}
				found[rp.GetFullName()] = struct{}{}
				if resp.Rate.Remaining == 0 {
					s := resp.Rate.Reset.Time.Sub(time.Now())
					log.Printf("quota exhausted, sleeping %v", s)
					time.Sleep(s)
				}
				all++
				break
			}
		}
	}
}

func getAllPages(q string) int {
	opt := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
		Sort:        "stars",
	}
	var last int
	log.Printf("q: %s", q)
	for {
		ctx := context.Background()
		repos, resp, err := client.Search.Repositories(ctx, q, opt)
		if err != nil {
			if _, ok := err.(*github.RateLimitError); ok {
				log.Println("hit rate limit")
				time.Sleep(10 * time.Second)
				continue
			} else {
				log.Fatal(err)
			}
		}
		if repos.GetIncompleteResults() {
			log.Printf("incomplete results, retrying with %s", q)
			continue
		}
		for i, r := range repos.Repositories {
			if i != 0 || last != 0 || q != "language:go" {
				if _, err = out.Write([]byte{',', '\n'}); err != nil {
					log.Fatal(err)
				}
			}
			if err = jsonEnc.Encode(r); err != nil {
				log.Fatal(err)
			}
			all++
			last = r.GetStargazersCount()
			found[r.GetFullName()] = struct{}{}
		}
		log.Printf("all: %d, last star count: %v, rate: %v", all, last, resp.Rate)
		if resp.NextPage == 0 {
			break
		}
		if resp.Rate.Remaining == 0 {
			s := resp.Rate.Reset.Time.Sub(time.Now())
			log.Printf("quota exhausted, sleeping %v", s)
			time.Sleep(s)
		}
		opt.Page = resp.NextPage
	}
	return last
}
