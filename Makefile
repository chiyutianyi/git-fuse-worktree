export GO111MODULE=on

all: clean git-worktree

REVISION := $(shell git describe --tags --match 'v*' --always --dirty 2>/dev/null)
REVISIONDATE := $(shell git log -1 --pretty=format:'%ad' --date short 2>/dev/null)
PKG := github.com/chiyutianyi/git-fuse-worktree
LDFLAGS = -s -w
ifneq ($(strip $(REVISION)),) # Use git clone
	LDFLAGS += -X $(PKG).revision=$(REVISION) \
		   -X $(PKG).revisionDate=$(REVISIONDATE)
endif

git-worktree: Makefile cmd/*.go pkg/*/*.go
	go build -ldflags="$(LDFLAGS)" -o git-worktree ./cmd

git-worktree-macos: Makefile cmd/*.go pkg/*/*.go
	go build -ldflags="$(LDFLAGS)" -o git-worktree-$(REVISION) ./cmd

git-worktree-linux: Makefile cmd/*.go pkg/*/*.go
	 GOOS=linux go build -ldflags="$(LDFLAGS)" -o git-worktree-$(REVISION).x86_64 ./cmd

git-worktree-linux-latest: Makefile cmd/*.go pkg/*/*.go
	 GOOS=linux go build -ldflags="$(LDFLAGS)" -o git-worktree-latest.x86_64 ./cmd

clean:
	rm -f git-worktree git-worktree-* git-worktree-*.x86_64