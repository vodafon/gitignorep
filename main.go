package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/go-github/github"
	"github.com/vodafon/swork"
	"golang.org/x/oauth2"
)

var (
	flagName        = flag.String("name", "", "org or user name")
	flagFile        = flag.String("file", ".gitignore", "git filename")
	flagAccessToken = flag.String("t", "", "access token. by default from env GITHUB_ACCESS_TOKEN")
	flagProcs       = flag.Int("procs", 10, "concurrency")
)

type Worker struct {
	client    *github.Client
	ownerName string
	filePath  string
}

func (obj *Worker) Process(line string) {
	parts := strings.Split(line, "/")
	if len(parts) != 2 {
		log.Fatalf("invalid repo/branch %q", line)
	}
	repo := parts[0]
	branch := parts[1]
	opt := &github.RepositoryContentGetOptions{
		Ref: branch,
	}
	ff, _, _, err := obj.client.Repositories.GetContents(context.Background(), obj.ownerName, repo, obj.filePath, opt)
	if err != nil {
		if strings.Contains(err.Error(), "404 ") {
			return
		}
		log.Fatalf("WARN: repo %q file fetch error: %v", repo, err)
		return
	}
	contentB64 := strings.ReplaceAll(*ff.Content, "\n", "")
	contentB, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		log.Printf("WARN: Repo: %q, decode content from base64 (%q) error: %v", repo, contentB64, err)
		return
	}
	content := string(contentB)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		fmt.Println(line)
	}
}

func main() {
	flag.Parse()
	if *flagName == "" || *flagFile == "" || *flagProcs < 1 {
		flag.PrintDefaults()
		os.Exit(1)
	}
	if *flagAccessToken == "" {
		*flagAccessToken = os.Getenv("GITHUB_ACCESS_TOKEN")
	}
	if *flagAccessToken == "" {
		log.Fatal("GitHub access token is empty. Please provide -t flag or set GITHUB_ACCESS_TOKEN env variable")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *flagAccessToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)
	opt := &github.RepositoryListOptions{
		Type: "public",
		Sort: "updated",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}
	repos, _, err := client.Repositories.List(context.Background(), *flagName, opt)
	if err != nil {
		log.Fatalf("get repositories error: %v", err)
	}
	worker := &Worker{
		client:    client,
		ownerName: *flagName,
		filePath:  *flagFile,
	}
	w := swork.NewWorkerGroup(*flagProcs, worker)

	for _, repo := range repos {
		if *repo.Fork {
			continue
		}
		w.StringC <- fmt.Sprintf("%s/%s", *repo.Name, *repo.DefaultBranch)
	}

	close(w.StringC)

	w.Wait()
}
