package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/github"
)

func main() {
	reposFile := flag.String("rep", "", "path of the repos.json")
	downloadDir := flag.String("d", "download", "path where repos should be downloaded")
	flag.Parse()
	*downloadDir = filepath.Join(*downloadDir, "github.com/") + string(filepath.Separator)
	inp, err := os.Open(*reposFile)
	if err != nil {
		log.Fatal(err)
	}
	defer inp.Close()
	var repos []github.Repository
	err = json.NewDecoder(inp).Decode(&repos)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%d repositories were loaded from %s", len(repos), *reposFile)
	for i, r := range repos {
		if r.GetFullName() == "" {
			log.Printf("ERROR: empty full name: %d", i)
		}
		if _, err := os.Stat(*downloadDir + r.GetFullName()); os.IsNotExist(err) {
			download(*downloadDir, r.GetArchiveURL(), r.GetFullName())
		} else {
			log.Printf("skipping %s: %v", r.GetFullName(), err)
		}
	}
}

var excludedDirs = []string{"vendor", "Godeps", "_vendor", "workspace", "_workspace"}

//https://api.github.com/repos/moby/moby/{archive_format}{/ref}
func download(ddir string, url string, repo string) {
	url = strings.Replace(url, "{archive_format}", "tarball", 1)
	url = strings.Replace(url, "{/ref}", "/master", 1)
	log.Printf("downloading: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("failed to download: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("bas status: %v", resp.StatusCode)
		return
	}
	archive := resp.Header["Content-Disposition"][0]
	if !strings.Contains(archive, "filename=") {
		log.Printf("cannot find filename: %v", resp.Header)
		return
	}
	archive = strings.Split(archive, "filename=")[1]
	af, err := os.Create(archive)
	if err != nil {
		log.Printf("failed to create file with name %s: %v", archive, err)
		return
	}
	_, err = io.Copy(af, resp.Body)
	af.Close()
	if err != nil {
		log.Printf("failed to save file with name %s: %v", archive, err)
		return
	}
	log.Printf("file %s saved", archive)
	af, err = os.Open(archive)
	if err != nil {
		log.Printf("failed to open archive: %v", err)
		return
	}
	err = untar(ddir+repo, af, excludedDirs)
	if err != nil {
		log.Printf("failed to untar archive: %v", err)
		return
	}
	log.Print("archive extracted to %s", ddir+repo)
	err = af.Close()
	if err != nil {
		log.Printf("failed to close archive: %v", err)
		return
	}
	err = os.Remove(archive)
	if err != nil {
		log.Printf("failed to remove archive: %v", err)
		return
	}
	log.Print("download finished")
}

// https://medium.com/@skdomino/taring-untaring-files-in-go-6b07cf56bc07
// untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
// added excluding directories
func untar(dst string, r io.Reader, excl []string) error {

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}
		if excluded(header.Typeflag == tar.TypeReg, header.Name, excl) {
			continue
		}
		ni := strings.Index(header.Name, "/")
		target := dst
		if ni != -1 {
			target = filepath.Join(dst, header.Name[ni:])
		}
		// the target location where the dir/file should be created
		// target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}

func excluded(file bool, path string, excl []string) bool {
	if file && filepath.Ext(path) != ".go" {
		return true
	}
	parts := strings.Split(path, string(filepath.Separator))
	for _, p := range parts {
		for _, e := range excl {
			if p == e {
				return true
			}
		}
	}
	return false
}
