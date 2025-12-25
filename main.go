package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func Init() *Repo {
	info, err := os.Stat(".tinygit")
	if err == nil && info.IsDir() {
		cwd, _ := os.Getwd()
		msg := fmt.Sprintf("Reinitialized existing tinygit repository in %s/%s\n", cwd, ".tinygit")
		fmt.Print(msg)
	} else if os.IsNotExist(err) {
		must(os.MkdirAll(".tinygit/objects", 0755))
		must(os.MkdirAll(".tinygit/refs/heads", 0755))
		head, err := os.Create(".tinygit/HEAD")
		must(err)
		_, err = head.WriteString("ref: refs/heads/main")
		must(err)
		fmt.Println("Initialized empty tinygit repository")
	}
	return &Repo{
		basePath: ".tinygit",
	}
}

type Object interface {
	Serialize() (n int, data string, err error)
}

type Blob struct {
	data string
}

func (b *Blob) Serialize() (n int, data string, err error) {
	return len([]byte(b.data)), b.data, nil
}

// func createObject()

func __getFullPath(relPath string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	absPath := fmt.Sprintf("%s/%s", cwd, relPath)
	return absPath, nil
}

type Repo struct {
	basePath string
}

func (r *Repo) Add(paths []string) {
	if len(paths) == 1 && paths[0] == "." {
		// recursively add all changed files
	}
	for _, path := range paths {
		absPath, _ := __getFullPath(path)
		f, _ := os.Open(absPath)
		defer f.Close()

		stat, _ := f.Stat()
		size := stat.Size()

		h := sha1.New()
		var compressed bytes.Buffer
		w := zlib.NewWriter(&compressed)

		header := fmt.Sprintf("blob %d\x00", size)
		h.Write([]byte(header))
		w.Write([]byte(header))

		io.Copy(io.MultiWriter(h, w), f)
		w.Close()

		hashStr := fmt.Sprintf("%x", h.Sum(nil))
		folderName := []byte(hashStr)[:2]
		fileName := []byte(hashStr)[2:]
		folderPath := filepath.Join(r.basePath, "objects", string(folderName))
		fmt.Println("folderPath : ", folderPath)
		filePath := filepath.Join(r.basePath, "objects", string(folderName), string(fileName))
		fmt.Println("filePath", filePath)
		_ = os.Mkdir(folderPath, 0755)
		_ = os.WriteFile(filePath, compressed.Bytes(), 0755)
	}
}

func findRepo() (*Repo, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, nil
	}

	for {
		gitPath := filepath.Join(dir, ".tinygit")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return &Repo{basePath: gitPath}, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// reached root
			return nil, fmt.Errorf("fatal: not a tinygit repository (or any of the parent directories): .tinygit")
		}
		dir = parent
	}
}

func main() {
	args := os.Args
	if len(args) <= 1 {
		fmt.Println("Please specify a subcommand arg")
		return
	}
	switch args[1] {
	case "init":
		Init()
	case "add":
		repo, err := findRepo()
		if err != nil {
			log.Fatal(err)
		}
		repo.Add(args[2:])
	}
}
