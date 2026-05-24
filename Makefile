APP     := fileshare
OUTDIR  := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all windows linux-amd64 linux-arm mac-arm mac-x86 clean

all: windows linux-amd64 linux-arm mac-arm mac-x86

windows:
	@mkdir -p $(OUTDIR)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(APP).exe .
	@echo "  built  $(OUTDIR)/$(APP)-windows-amd64.exe"

linux-amd64:
	@mkdir -p $(OUTDIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(APP)-linux-amd64 .
	@echo "  built  $(OUTDIR)/$(APP)-linux-amd64"

linux-arm:
	@mkdir -p $(OUTDIR)
	GOOS=linux GOARCH=arm GOARM=7 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(APP)-linux-arm .
	@echo "  built  $(OUTDIR)/$(APP)-linux-arm7"

mac-arm:
	@mkdir -p $(OUTDIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(APP)-mac-arm .
	@echo "  built  $(OUTDIR)/$(APP)-mac-arm64"

mac-x86:
	@mkdir -p $(OUTDIR)
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(APP)-mac-amd64 .
	@echo "  built  $(OUTDIR)/$(APP)-mac-amd64"

clean:
	rm -rf $(OUTDIR)
	@echo "  cleaned $(OUTDIR)/"
