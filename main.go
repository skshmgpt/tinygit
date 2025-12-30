package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	basePath     string
	objDirPath   string
	refsDirPath  string
	HEAD         string
	indxFilePath string
}

func (r *Repo) __writeToIndx(hash, path string) error {
	indxF, err := os.OpenFile(r.indxFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer indxF.Close()

	_, err = fmt.Fprintf(indxF, "100644 %s %s\n", hash, path)
	if err != nil {
		return err
	}
	return err
}

// TODO: implement directory traversal and . resolution
// TODO: before hashing, check index, add more meta in index, to check if alr added

func (r *Repo) __createBlobFromFile(file string) (hashStr string) {
	absPath, _ := __getFullPath(file)
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

	hashStr = fmt.Sprintf("%x", h.Sum(nil))
	folderName := []byte(hashStr)[:2]
	fileName := []byte(hashStr)[2:]
	folder_path := filepath.Join(r.objDirPath, string(folderName))
	file_path := filepath.Join(r.objDirPath, string(folderName), string(fileName))
	_ = os.Mkdir(folder_path, 0755)
	_ = os.WriteFile(file_path, compressed.Bytes(), 0755)
	return hashStr
}

func (r *Repo) __getAllFilesinDir(dirPath string) []string {
	// walk the directory and keep adding files
	var files []string
	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && (info.Name() == ".git" || info.Name() == ".tinygit") {
			return filepath.SkipDir
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files
}

func (r *Repo) Add(paths []string) {
	var filesToAdd []string
	if len(paths) == 1 && paths[0] == "." {
		filesToAdd = r.__getAllFilesinDir(".")
	} else {
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil {
				log.Fatal(err)
			}
			if info.IsDir() {
				files := r.__getAllFilesinDir(path)
				filesToAdd = append(filesToAdd, files...)
			} else {
				filesToAdd = append(filesToAdd, path)
			}
		}
	}
	for _, path := range filesToAdd {
		hashStr := r.__createBlobFromFile(path)
		err := r.__writeToIndx(hashStr, path)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func (r *Repo) Branch(branchName string) {
	currCommit := r.__getCurrCommit()
	if currCommit == "" {
		log.Fatal("no commits yet")
	}
	os.WriteFile(filepath.Join(r.refsDirPath, "heads", branchName), []byte(currCommit), 0644)
	log.Printf("Created new branch %s", branchName)

}

func (r *Repo) __resolveTargetToCommit(target string) string {
	branchPath := filepath.Join(r.refsDirPath, "heads", target)
	if data, err := os.ReadFile(branchPath); err == nil {
		return strings.TrimSpace(string(data))
	}
	return target
}

func (r *Repo) __resolveTreeHashFromCommitHash(commitHash string) (string, error) {
	commitBody, err := r.catFile("-p", commitHash)
	if err != nil {
		return "", err
	}
	lines := strings.Split(commitBody, "\n")
	treeLine := lines[0]
	treeHash := strings.Fields(treeLine)[1]
	return treeHash, nil
}

func (r *Repo) __getTreeContent(treeHash string) []string {
	body, _ := r.catFile("-p", treeHash)
	lines := strings.Split(strings.TrimSpace(body), "\n")
	return lines
}

func (r *Repo) __clearWorkingDir() {
	repoRoot := filepath.Dir(r.basePath)
	entries, _ := os.ReadDir(repoRoot)

	for _, entry := range entries {
		if entry.Name() == ".tinygit" || entry.Name() == ".git" {
			continue
		}
		path := filepath.Join(repoRoot, entry.Name())
		os.RemoveAll(path)
	}
}

func (r *Repo) Checkout(target string) {
	// commit or branch hash
	// we need to move HEAD to that ref, and reconstruct the directory
	commitHash := r.__resolveTargetToCommit(target)
	treeHash, err := r.__resolveTreeHashFromCommitHash(commitHash)
	if err != nil {
		log.Fatal(err)
		return
	}

	r.__clearWorkingDir()

	treeContent := r.__getTreeContent(treeHash)

	for _, entry := range treeContent {
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "\t")
		if len(parts) != 2 {
			continue
		}
		meta := parts[0]
		filePath := parts[1]
		metaParts := strings.Fields(meta)
		if len(metaParts) < 3 {
			continue
		}
		fileHash := metaParts[2]

		fileContent, err := r.catFile("-p", fileHash)
		if err != nil {
			log.Fatal(err)
			return
		}
		repoRoot := filepath.Dir(r.basePath)
		filePathFromRoot := filepath.Join(repoRoot, filePath)

		os.MkdirAll(filepath.Dir(filePathFromRoot), 0755)
		os.WriteFile(filePathFromRoot, []byte(fileContent), 0644)
	}
	r.__updateHEAD(target, commitHash)

}

func (r *Repo) __updateHEAD(target, commitHash string) {
	// Check if target is a branch (does refs/heads/<target> exist?)
	branchPath := filepath.Join(r.refsDirPath, "heads", target)

	if _, err := os.Stat(branchPath); err == nil {
		// Target is a branch - point HEAD to the branch reference
		headContent := fmt.Sprintf("ref: refs/heads/%s", target)
		os.WriteFile(r.HEAD, []byte(headContent), 0644)
	} else {
		// Target is a commit hash - detached HEAD state
		os.WriteFile(r.HEAD, []byte(commitHash), 0644)
	}
}

func (r *Repo) catFile(flag, fileHash string) (string, error) {
	folderName := []byte(fileHash)[:2]
	fileName := []byte(fileHash)[2:]
	filePath := filepath.Join(r.basePath, "objects", string(folderName), string(fileName))

	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	zR, err := zlib.NewReader(f)
	if err != nil {
		return "", err
	}
	defer zR.Close()

	decompressed, err := io.ReadAll(zR)
	if err != nil {
		return "", err
	}
	content := strings.SplitN(string(decompressed), "\x00", 2)
	header := content[0]
	body := content[1]

	hParts := strings.Fields(header)

	switch flag {
	case "-p":
		return body, nil
	case "-t":
		return hParts[0], nil
	case "-s":
		return hParts[1], nil
	}
	return "", nil
}

func (r *Repo) createTreeFromIdx() string {

	idx, err := os.Open(r.indxFilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer idx.Close()
	scanner := bufio.NewScanner(idx)

	var treeBuf bytes.Buffer
	for scanner.Scan() {
		entry := scanner.Text()
		content := strings.Split(entry, " ")
		mode := content[0]
		hash := content[1]
		filePath := content[2]

		var objType string
		switch mode {
		case "040000":
			objType = "tree"
		case "100644", "100755":
			objType = "blob"
		}

		fmt.Fprintf(&treeBuf, "%s %s %s\t%s\n", mode, objType, hash, filePath)
	}
	treeContent := treeBuf.String()
	treeHeader := fmt.Sprintf("tree %d\x00", len(treeContent))
	tree := treeHeader + treeContent

	treeHashB := sha1.Sum([]byte(tree))
	treeHash := fmt.Sprintf("%x", treeHashB)

	var compressed bytes.Buffer
	zW := zlib.NewWriter(&compressed)
	zW.Write([]byte(tree))
	zW.Close()

	compressedTree := compressed.Bytes()

	folder := treeHash[:2]
	os.Mkdir(filepath.Join(r.objDirPath, folder), 0755)

	file := treeHash[2:]
	filePath := filepath.Join(r.objDirPath, folder, file)
	os.WriteFile(filePath, compressedTree, 0644)
	return treeHash
}

func (r *Repo) __getCurrBranch() string {
	headData, _ := os.ReadFile(r.HEAD)
	branchRef := strings.TrimSpace(string(headData))
	branchRef = strings.TrimPrefix(branchRef, "ref: ")

	// HEAD gives the current branch reference
	// now we will read what commit current branch is on

	currBranchPath := filepath.Join(r.basePath, branchRef)
	return currBranchPath

}

func (r *Repo) __getCurrCommit() string {
	currCommit := ""

	currBranchPath := r.__getCurrBranch()
	_, err := os.Stat(currBranchPath)
	if os.IsNotExist(err) {
		// no branch exist/ this is first commit -> no parent commit
	} else {
		currBranch_f, _ := os.Open(currBranchPath)
		currBranch_b, _ := io.ReadAll(currBranch_f)
		currCommit = string(currBranch_b)
	}
	return currCommit
}
func (r *Repo) __updateRef(newCommitHash string) {
	currentBranchPath := r.__getCurrBranch()
	os.WriteFile(currentBranchPath, []byte(newCommitHash), 0644)
}

func (r *Repo) Commit(msg string) {

	treeHash := r.createTreeFromIdx()

	currCommit := r.__getCurrCommit()
	var commitBody string

	if currCommit == "" {
		commitBody = fmt.Sprintf("tree %s\n", treeHash)
	} else {
		commitBody = fmt.Sprintf("tree %s\nparent %s\n", treeHash, currCommit)
	}

	commitBody += msg

	commit_header := fmt.Sprintf("commit %d\x00", len(commitBody))

	commit := commit_header + commitBody

	var compressedCommit bytes.Buffer
	zW := zlib.NewWriter(&compressedCommit)
	zW.Write([]byte(commit))
	zW.Close()

	commitHash := sha1.Sum([]byte(commit))
	commitHashStr := fmt.Sprintf("%x", commitHash)

	folder := commitHashStr[:2]
	os.Mkdir(filepath.Join(r.objDirPath, folder), 0755)

	file := commitHashStr[2:]
	file_path := filepath.Join(r.objDirPath, folder, file)
	os.WriteFile(file_path, compressedCommit.Bytes(), 0644)
	r.__updateRef(commitHashStr)
}

func __findRepo() (*Repo, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, nil
	}

	for {
		gitPath := filepath.Join(dir, ".tinygit")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			indxPath := fmt.Sprintf("%s/index", gitPath)
			head := fmt.Sprintf("%s/HEAD", gitPath)
			objDirPath := fmt.Sprintf("%s/objects", gitPath)
			refDirPath := fmt.Sprintf("%s/refs", gitPath)
			return &Repo{basePath: gitPath, indxFilePath: indxPath, HEAD: head, objDirPath: objDirPath, refsDirPath: refDirPath}, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// reached root
			return nil, fmt.Errorf("fatal: not a tinygit repository (or any of the parent directories): .tinygit")
		}
		dir = parent
	}
}

func GetRepo() *Repo {
	repo, err := __findRepo()
	if err != nil {
		log.Fatal(err)
	}
	return repo
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
		repo := GetRepo()
		repo.Add(args[2:])
	case "cat-file":
		repo := GetRepo()
		res, err := repo.catFile(args[2], args[3])
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("\n%s\n", res)
	case "commit":
		repo := GetRepo()
		repo.Commit(args[3])
	case "branch":
		repo := GetRepo()
		repo.Branch(args[3])
	case "checkout":
		repo := GetRepo()
		repo.Checkout(args[2])
	}
}
