package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/KyleBanks/depth"
)

func main() {
	rp := flag.String("r", "", "path of the downloaded github repos")
	of := flag.String("o", "deps.csv", "output file")
	flag.Parse()
	out, err := os.Create(*of)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	gp := os.Getenv("GOPATH")
	err = filepath.Walk(*rp, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("prevent panic by handling failure accessing a path %q: %v\n", path, err)
			return err
		}
		if !info.IsDir() {
			return nil
		}
		var t depth.Tree
		t.MaxDepth = 1
		p := strings.Replace(path, gp+string(os.PathSeparator)+"src"+string(os.PathSeparator), "", 1)
		if !validSrc(p) {
			return nil
		}
		err = t.Resolve(p)
		if err != nil {
			log.Printf("error with %s: %v", p, err)
			return nil
		}
		for _, d := range t.Root.Deps {
			out.WriteString(fmt.Sprintf("%s,%s\n", p, d.Name))
		}
		return nil
	})
	if err != nil {
		log.Printf("error walking repos in %s: %v\n", *rp, err)
		return
	}
}

var invalidSrc = []string{"vendor", "_vendor", "workspace", "_workspace", "Godeps"}

func validSrc(src string) bool {
	dirs := strings.Split(src, string(os.PathSeparator))
	gh := 0
	for _, d := range dirs {
		if d == "github.com" {
			gh++
			continue
		}
		for _, id := range invalidSrc {
			if d == id {
				return false
			}
		}
	}
	return gh == 1
}
