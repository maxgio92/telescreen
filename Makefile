BIN     := $(HOME)/.local/bin
STATE   := $(if $(XDG_STATE_HOME),$(XDG_STATE_HOME),$(HOME)/.local/state)/recdep
UNITS   := $(HOME)/.config/systemd/user
SKILLS  := $(HOME)/.claude/skills
REPO    := $(CURDIR)

.PHONY: build
build:
	go build -o $(BIN)/telescreen .
	go build -o $(BIN)/thinkpol ./cmd/thinkpol

.PHONY: minitrue
minitrue:
	@mkdir -p $(UNITS) $(SKILLS) $(STATE)/tube $(STATE)/desk $(STATE)/upsub $(STATE)/files
	install -m 0755 minitrue/minitrue $(BIN)/minitrue
	ln -sfn $(REPO)/minitrue $(SKILLS)/minitrue
	ln -sf $(REPO)/minitrue/minitrue.service $(UNITS)/minitrue.service
	ln -sf $(REPO)/minitrue/minitrue.timer $(UNITS)/minitrue.timer
	systemctl --user daemon-reload
	systemctl --user enable --now minitrue.timer
	@echo "minitrue timer enabled. Set ~/.config/minitrue.env for identity (SLACK_USER_ID, GH_LOGIN, LINEAR_ASSIGNEE, REPO)."

.PHONY: speakwrite
speakwrite:
	@mkdir -p $(UNITS) $(SKILLS) $(STATE)/intents
	install -m 0755 speakwrite/speakwrite $(BIN)/speakwrite
	ln -sfn $(REPO)/speakwrite $(SKILLS)/speakwrite
	ln -sf $(REPO)/speakwrite/speakwrite.service $(UNITS)/speakwrite.service
	ln -sf $(REPO)/speakwrite/speakwrite.path $(UNITS)/speakwrite.path
	systemctl --user daemon-reload
	systemctl --user enable --now speakwrite.path
	@echo "speakwrite path unit enabled. Intents in $(STATE)/intents trigger the drafting runner."

.PHONY: thinkpol
thinkpol:
	@mkdir -p $(UNITS) $(STATE)/intents
	ln -sf $(REPO)/thinkpol/thinkpol.service $(UNITS)/thinkpol.service
	ln -sf $(REPO)/thinkpol/thinkpol.path $(UNITS)/thinkpol.path
	systemctl --user daemon-reload
	systemctl --user enable --now thinkpol.path
	@echo "thinkpol path unit enabled. Publish approvals in $(STATE)/intents trigger the actor."

.PHONY: install
install: build minitrue speakwrite thinkpol

.PHONY: test
test:
	go vet ./...
	go test ./...
