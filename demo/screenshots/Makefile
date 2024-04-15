
TAPES := $(wildcard atmos-*.tape)
ALL_TAPES :=$(shell (while read -r line; do echo $${line}.tape | tr ' ' '-' | sed 's/---/--/g'; done)<commands.txt)
SCREENSHOTS := $(TAPES:.tape=.png)
INSTALL_PATH := ../../website/static/img/screenshots
# We need to store the artifacts somehwere consistently
# We need to run the demo from the root of the project
# Source is relative to CWD

# Set Working Directory (rejected) https://github.com/charmbracelet/vhs/issues/19
# Set CWD (open) https://github.com/charmbracelet/vhs/issues/320
# Hide doesn't hide https://github.com/charmbracelet/vhs/issues/130
# Fix hide in https://github.com/charmbracelet/vhs/pull/304

# Write to /website/static/img/screenshots

all: $(SCREENSHOTS)

install: $(SCREENSHOTS)
	@echo "Installing screenshots to $(INSTALL_PATH)"
	@mv -f $(SCREENSHOTS) $(INSTALL_PATH)

atmos-%.png: atmos-%.tape
	@echo "Screenshotting $< to $@"
	@#vhs $<
	docker run --rm --name $< --env-file=.dockerenv -it -v $(PWD)/../../examples:/vhs/examples -v $(PWD):/vhs -w /vhs vhs $<
	@rm -f out.gif

clean:
	rm -f *.gif *.mp4 *.png *.jpg *.ascii

realclean: clean
	rm -f *.tape

%.ansi:
	ls -al --color=force > $@

%.svg: %.ansi
	@echo "Converting $< to $@"
	aha --css "file://./term.css" < $< | sed '/<body>/r term.html' | wkhtmltoimage --allow './' --enable-local-file-access - $@

%.tape: commands.txt
	@while read -r line; do \
		cmd=$$(echo $$line | tr ' ' '-' | sed 's/---/--/g'); \
		if [ "$$cmd.tape" != "$@" ]; then \
			continue; \
		fi; \
		echo "Creating $$cmd.tape"; \
		echo "# Demo of \`$$line\`" > $$cmd.tape; \
		echo "Source sources/style.tape" >> $$cmd.tape; \
		echo "Source sources/atmos-stacks.tape" >> $$cmd.tape; \
		echo "Output $$cmd.gif" >> $$cmd.tape; \
		echo "Type \"$$line\" Enter Sleep 2500ms " >> $$cmd.tape; \
		echo "Screenshot $$cmd.png" >> $$cmd.tape; \
	done < $<

.PHONY: tapes
tapes: $(ALL_TAPES)
