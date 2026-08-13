BIN     := $(HOME)/.local/bin
STATE   := $(if $(XDG_STATE_HOME),$(XDG_STATE_HOME),$(HOME)/.local/state)/recdep
UNITS   := $(HOME)/.config/systemd/user
SKILLS  := $(HOME)/.claude/skills
REPO    := $(CURDIR)

.PHONY: build
build:
	go build -o $(BIN)/telescreen .

.PHONY: minitrue
minitrue:
	@mkdir -p $(UNITS) $(SKILLS) $(STATE)/inbox $(STATE)/todo $(STATE)/waiting $(STATE)/archive
	install -m 0755 minitrue/minitrue $(BIN)/minitrue
	ln -sfn $(REPO)/minitrue $(SKILLS)/minitrue
	ln -sf $(REPO)/minitrue/minitrue.service $(UNITS)/minitrue.service
	ln -sf $(REPO)/minitrue/minitrue.timer $(UNITS)/minitrue.timer
	systemctl --user daemon-reload
	systemctl --user enable --now minitrue.timer
	@echo "minitrue timer enabled. Set ~/.config/minitrue.env for identity (SLACK_USER_ID, GH_LOGIN, LINEAR_ASSIGNEE, REPO)."

.PHONY: install
install: build minitrue

.PHONY: test
test:
	go vet ./...
	go test ./...
